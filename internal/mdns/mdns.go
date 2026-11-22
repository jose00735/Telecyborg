// Package mdns advertises the TeleCyborg phone-sync hub as a DNS-SD service
// (_telecyborg._tcp.local) so phones find the PC without typing an IP. Standard library only.
// Adapted from Macawlink (github.com/jose00735/macawlink), same author.
package mdns

import (
	"encoding/binary"
	"errors"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	ServiceType = "_telecyborg._tcp.local."
	metaQuery   = "_services._dns-sd._udp.local."

	typeA   = 1
	typePTR = 12
	typeTXT = 16
	typeSRV = 33
	typeANY = 255

	classIN    = 1
	cacheFlush = 0x8000
)

var group = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// Service describes what we advertise.
type Service struct {
	Instance string            // e.g. "TeleCyborg on DESKTOP-42"
	Host     string            // e.g. "telecyborg-desktop-42" (".local." is appended)
	Port     int               // TLS port
	TXT      map[string]string // e.g. v=1, name=…, fp=…
}

func (s Service) instanceName() string { return escapeLabel(s.Instance) + "." + ServiceType }
func (s Service) hostName() string     { return s.Host + ".local." }

// Responder answers queries on every IPv4 multicast interface.
type Responder struct {
	svc   Service
	mu    sync.Mutex
	socks map[string]*net.UDPConn // by interface name
	done  chan struct{}
}

func Start(svc Service) *Responder {
	r := &Responder{svc: svc, socks: map[string]*net.UDPConn{}, done: make(chan struct{})}
	r.refresh()
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for i := 0; ; i++ {
			select {
			case <-r.done:
				return
			case <-t.C:
				r.refresh()
				if i%2 == 1 {
					r.announce(120)
				}
			}
		}
	}()
	// RFC 6762 §8.3: announce a few times at startup.
	go func() {
		for _, d := range []time.Duration{0, time.Second, 2 * time.Second} {
			select {
			case <-r.done:
				return
			case <-time.After(d):
				r.announce(120)
			}
		}
	}()
	return r
}

func (r *Responder) Stop() {
	r.announce(0) // goodbye
	close(r.done)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.socks {
		c.Close()
	}
}

func (r *Responder) refresh() {
	ifs, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, ifi := range ifs {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		if len(ipv4s(&ifi)) == 0 {
			continue
		}
		r.mu.Lock()
		_, have := r.socks[ifi.Name]
		r.mu.Unlock()
		if have {
			continue
		}
		ifc := ifi
		c, err := net.ListenMulticastUDP("udp4", &ifc, group)
		if err != nil {
			log.Printf("mdns: %s: %v", ifi.Name, err)
			continue
		}
		r.mu.Lock()
		r.socks[ifi.Name] = c
		r.mu.Unlock()
		log.Printf("mdns: advertising %q on %s %v", r.svc.Instance, ifi.Name, ipv4s(&ifc))
		go r.serve(c, &ifc)
	}
}

func (r *Responder) serve(c *net.UDPConn, ifi *net.Interface) {
	buf := make([]byte, 9000)
	for {
		n, src, err := c.ReadFromUDP(buf)
		if err != nil {
			r.mu.Lock()
			delete(r.socks, ifi.Name)
			r.mu.Unlock()
			return
		}
		resp, unicast := Respond(buf[:n], r.svc, ipv4s(ifi), 120)
		if resp == nil {
			continue
		}
		if unicast || src.Port != 5353 {
			c.WriteToUDP(resp, src) // legacy / QU query: answer the asker directly
		}
		if src.Port == 5353 {
			c.WriteToUDP(resp, group)
		}
	}
}

func (r *Responder) announce(ttl uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, c := range r.socks {
		ifi, err := net.InterfaceByName(name)
		if err != nil {
			continue
		}
		msg := response(0, r.svc, ipv4s(ifi), ttl, true, true)
		c.WriteToUDP(msg, group)
	}
}

func ipv4s(ifi *net.Interface) []net.IP {
	addrs, _ := ifi.Addrs()
	var out []net.IP
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			if ip4 := ipn.IP.To4(); ip4 != nil && !ip4.IsLinkLocalUnicast() {
				out = append(out, ip4)
			}
		}
	}
	return out
}

// Respond builds an answer to an mDNS query packet, or returns nil if it isn't for us.
func Respond(query []byte, svc Service, ips []net.IP, ttl uint32) (resp []byte, unicast bool) {
	id, qs, err := parseQuery(query)
	if err != nil {
		return nil, false
	}
	wantPTR, wantInst, wantHost, wantMeta := false, false, false, false
	for _, q := range qs {
		name := strings.ToLower(q.name)
		unicast = unicast || q.class&0x8000 != 0
		switch {
		case name == ServiceType && (q.typ == typePTR || q.typ == typeANY):
			wantPTR = true
		case name == strings.ToLower(svc.instanceName()) && (q.typ == typeSRV || q.typ == typeTXT || q.typ == typeANY):
			wantInst = true
		case name == strings.ToLower(svc.hostName()) && (q.typ == typeA || q.typ == typeANY):
			wantHost = true
		case name == metaQuery && q.typ == typePTR:
			wantMeta = true
		}
	}
	switch {
	case wantPTR || wantInst:
		return response(id, svc, ips, ttl, true, true), unicast
	case wantHost:
		return response(id, svc, ips, ttl, false, false), unicast
	case wantMeta:
		var b builder
		b.header(id, 1)
		b.record(metaQuery, typePTR, classIN, 4500, encodeName(ServiceType))
		return b.bytes(), unicast
	}
	return nil, false
}

func response(id uint16, svc Service, ips []net.IP, ttl uint32, ptr, srvtxt bool) []byte {
	var b builder
	count := len(ips)
	if ptr {
		count++
	}
	if srvtxt {
		count += 2
	}
	b.header(id, count)
	if ptr {
		b.record(ServiceType, typePTR, classIN, ttl*37, encodeName(svc.instanceName()))
	}
	if srvtxt {
		srv := make([]byte, 6)
		binary.BigEndian.PutUint16(srv[4:], uint16(svc.Port))
		b.record(svc.instanceName(), typeSRV, classIN|cacheFlush, ttl, append(srv, encodeName(svc.hostName())...))
		var txt []byte
		for _, k := range sortedKeys(svc.TXT) {
			kv := k + "=" + svc.TXT[k]
			if len(kv) > 255 {
				kv = kv[:255]
			}
			txt = append(txt, byte(len(kv)))
			txt = append(txt, kv...)
		}
		if len(txt) == 0 {
			txt = []byte{0}
		}
		b.record(svc.instanceName(), typeTXT, classIN|cacheFlush, ttl*37, txt)
	}
	for _, ip := range ips {
		b.record(svc.hostName(), typeA, classIN|cacheFlush, ttl, ip.To4())
	}
	return b.bytes()
}

// ---- wire helpers ----

type question struct {
	name       string
	typ, class uint16
}

func parseQuery(b []byte) (uint16, []question, error) {
	if len(b) < 12 {
		return 0, nil, errors.New("short")
	}
	id := binary.BigEndian.Uint16(b)
	flags := binary.BigEndian.Uint16(b[2:])
	if flags&0x8000 != 0 {
		return 0, nil, errors.New("not a query")
	}
	qd := int(binary.BigEndian.Uint16(b[4:]))
	off := 12
	var qs []question
	for i := 0; i < qd; i++ {
		name, n, err := readName(b, off, 0)
		if err != nil || n+4 > len(b) {
			return 0, nil, errors.New("bad question")
		}
		qs = append(qs, question{name, binary.BigEndian.Uint16(b[n:]), binary.BigEndian.Uint16(b[n+2:])})
		off = n + 4
	}
	return id, qs, nil
}

// readName decodes a (possibly compressed) name starting at off; returns the name and the
// offset just past it in the original position.
func readName(b []byte, off, depth int) (string, int, error) {
	if depth > 10 {
		return "", 0, errors.New("loop")
	}
	var labels []string
	for {
		if off >= len(b) {
			return "", 0, errors.New("overrun")
		}
		l := int(b[off])
		switch {
		case l == 0:
			return strings.Join(labels, ".") + ".", off + 1, nil
		case l&0xC0 == 0xC0:
			if off+1 >= len(b) {
				return "", 0, errors.New("overrun")
			}
			ptr := int(binary.BigEndian.Uint16(b[off:]) & 0x3FFF)
			rest, _, err := readName(b, ptr, depth+1)
			if err != nil {
				return "", 0, err
			}
			if len(labels) > 0 {
				rest = strings.Join(labels, ".") + "." + rest
			}
			return rest, off + 2, nil
		default:
			if off+1+l > len(b) {
				return "", 0, errors.New("overrun")
			}
			labels = append(labels, strings.ReplaceAll(string(b[off+1:off+1+l]), ".", "\\."))
			off += 1 + l
		}
	}
}

// encodeName writes a name; "\." inside the first label (instance names) is a literal dot.
func encodeName(name string) []byte {
	var out []byte
	name = strings.TrimSuffix(name, ".")
	var label []byte
	flush := func() {
		if len(label) > 63 {
			label = label[:63]
		}
		out = append(out, byte(len(label)))
		out = append(out, label...)
		label = label[:0]
	}
	for i := 0; i < len(name); i++ {
		switch {
		case name[i] == '\\' && i+1 < len(name):
			label = append(label, name[i+1])
			i++
		case name[i] == '.':
			flush()
		default:
			label = append(label, name[i])
		}
	}
	flush()
	return append(out, 0)
}

func escapeLabel(s string) string { return strings.ReplaceAll(s, ".", "\\.") }

type builder struct{ buf []byte }

func (b *builder) header(id uint16, answers int) {
	b.buf = make([]byte, 12, 512)
	binary.BigEndian.PutUint16(b.buf, id)
	binary.BigEndian.PutUint16(b.buf[2:], 0x8400) // response, authoritative
	binary.BigEndian.PutUint16(b.buf[6:], uint16(answers))
}

func (b *builder) record(name string, typ, class uint16, ttl uint32, rdata []byte) {
	b.buf = append(b.buf, encodeName(name)...)
	var f [10]byte
	binary.BigEndian.PutUint16(f[0:], typ)
	binary.BigEndian.PutUint16(f[2:], class)
	binary.BigEndian.PutUint32(f[4:], ttl)
	binary.BigEndian.PutUint16(f[8:], uint16(len(rdata)))
	b.buf = append(b.buf, f[:]...)
	b.buf = append(b.buf, rdata...)
}

func (b *builder) bytes() []byte { return b.buf }

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// HostLabel turns a computer name into a valid DNS host label.
func HostLabel(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "pc"
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return "telecyborg-" + out
}
