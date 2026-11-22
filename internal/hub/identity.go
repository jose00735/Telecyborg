package hub

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Identity is the PC's long-lived self-signed TLS certificate (ECDSA P-256).
type Identity struct {
	Cert        tls.Certificate
	Fingerprint []byte // SHA-256 of the leaf DER; phones pin it
}

// LoadOrCreateIdentity reads dir/tls.pem or generates a certificate.
func LoadOrCreateIdentity(dir, name string) (*Identity, error) {
	p := filepath.Join(dir, "tls.pem")
	if b, err := os.ReadFile(p); err == nil {
		if c, err := tls.X509KeyPair(b, b); err == nil && len(c.Certificate) > 0 {
			return identity(c), nil
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "TeleCyborg " + name, Organization: []string{"TeleCyborg"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(20, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	out := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	out = append(out, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb})...)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, out, 0o600); err != nil {
		return nil, err
	}
	c, err := tls.X509KeyPair(out, out)
	if err != nil {
		return nil, err
	}
	return identity(c), nil
}

func identity(c tls.Certificate) *Identity {
	sum := sha256.Sum256(c.Certificate[0])
	return &Identity{Cert: c, Fingerprint: sum[:]}
}

// ShortFP tells PCs apart in the phone's list.
func (id *Identity) ShortFP() string { return hex.EncodeToString(id.Fingerprint[:4]) }

// TLSConfig: TLS 1.3 only.
func (id *Identity) TLSConfig() *tls.Config {
	return &tls.Config{Certificates: []tls.Certificate{id.Cert}, MinVersion: tls.VersionTLS13}
}

// SAS is the 6-digit code both screens show during pairing. certFP is the SHA-256 of the PC
// certificate as each side sees it, so a man in the middle makes the two codes differ.
// Same construction as Macawlink's Wi-Fi pairing, with its own label.
func SAS(certFP, pcNonce, phoneNonce []byte) string {
	h := sha256.New()
	h.Write([]byte("telecyborg-pair-v1"))
	h.Write(certFP)
	h.Write(pcNonce)
	h.Write(phoneNonce)
	sum := h.Sum(nil)
	return fmt.Sprintf("%06d", binary.BigEndian.Uint32(sum[:4])%1000000)
}

// Commit is the PC's commitment to its nonce: hex SHA-256(nonce).
func Commit(nonce []byte) string {
	s := sha256.Sum256(nonce)
	return hex.EncodeToString(s[:])
}

// Phone is one paired device.
type Phone struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TokenHash string    `json:"token_hash,omitempty"`
	PairedAt  time.Time `json:"paired_at"`
	LastSync  time.Time `json:"last_sync"`
}

// Phones persists paired phones in dir/phones.json (token hashes only).
type Phones struct {
	mu     sync.Mutex
	path   string
	phones map[string]*Phone
}

func openPhones(dir string) *Phones {
	s := &Phones{phones: map[string]*Phone{}, path: filepath.Join(dir, "phones.json")}
	if b, err := os.ReadFile(s.path); err == nil {
		var list []*Phone
		if json.Unmarshal(b, &list) == nil {
			for _, p := range list {
				s.phones[p.ID] = p
			}
		}
	}
	return s
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// Check validates a phone's token and records the visit.
func (s *Phones) Check(phoneID, token string) bool {
	if phoneID == "" || token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.phones[phoneID]
	if !ok || subtle.ConstantTimeCompare([]byte(p.TokenHash), []byte(hashToken(token))) != 1 {
		return false
	}
	p.LastSync = time.Now()
	s.saveLocked()
	return true
}

// Add pairs a phone and returns its token.
func (s *Phones) Add(phoneID, name string) (string, error) {
	if phoneID == "" {
		return "", errors.New("empty phone id")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.phones[phoneID] = &Phone{ID: phoneID, Name: name, TokenHash: hashToken(tok), PairedAt: now, LastSync: now}
	return tok, s.saveLocked()
}

// Remove forgets a phone.
func (s *Phones) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.phones, id)
	s.saveLocked()
}

// List returns paired phones without token hashes.
func (s *Phones) List() []Phone {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Phone, 0, len(s.phones))
	for _, p := range s.phones {
		q := *p
		q.TokenHash = ""
		out = append(out, q)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PairedAt.Before(out[j].PairedAt) })
	return out
}

func (s *Phones) saveLocked() error {
	list := make([]*Phone, 0, len(s.phones))
	for _, p := range s.phones {
		list = append(list, p)
	}
	b, _ := json.MarshalIndent(list, "", "  ")
	_ = os.MkdirAll(filepath.Dir(s.path), 0o700)
	return os.WriteFile(s.path, b, 0o600)
}
