package mdns

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
)

func query(name string, typ uint16) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b[4:], 1)
	b = append(b, encodeName(name)...)
	return append(b, byte(typ>>8), byte(typ), 0, 1)
}

type rr struct {
	name       string
	typ, class uint16
	rdata      []byte
	off        int
}

func parseAnswers(t *testing.T, b []byte) []rr {
	t.Helper()
	an := int(binary.BigEndian.Uint16(b[6:]))
	off := 12
	var out []rr
	for i := 0; i < an; i++ {
		name, n, err := readName(b, off, 0)
		if err != nil {
			t.Fatal(err)
		}
		typ, class := binary.BigEndian.Uint16(b[n:]), binary.BigEndian.Uint16(b[n+2:])
		l := int(binary.BigEndian.Uint16(b[n+8:]))
		out = append(out, rr{name, typ, class, b[n+10 : n+10+l], n + 10})
		off = n + 10 + l
	}
	return out
}

func TestRespondPTR(t *testing.T) {
	svc := Service{Instance: "TeleCyborg on DESKTOP-42", Host: HostLabel("DESKTOP-42"), Port: 47802,
		TXT: map[string]string{"v": "1", "fp": "a1b2c3d4", "name": "DESKTOP-42"}}
	ips := []net.IP{net.IPv4(192, 168, 1, 20)}
	resp, _ := Respond(query(ServiceType, typePTR), svc, ips, 120)
	if resp == nil {
		t.Fatal("no response")
	}
	got := parseAnswers(t, resp)
	if len(got) != 4 {
		t.Fatalf("answers = %d", len(got))
	}
	ptrTarget, _, _ := readName(resp, got[0].off, 0)
	if got[0].typ != typePTR || ptrTarget != "TeleCyborg on DESKTOP-42._telecyborg._tcp.local." {
		t.Fatalf("ptr -> %q", ptrTarget)
	}
	srv := got[1]
	host, _, _ := readName(resp, srv.off+6, 0)
	if srv.typ != typeSRV || binary.BigEndian.Uint16(srv.rdata[4:]) != 47802 || host != "telecyborg-desktop-42.local." {
		t.Fatalf("srv port=%d host=%q", binary.BigEndian.Uint16(srv.rdata[4:]), host)
	}
	if !strings.Contains(string(got[2].rdata), "fp=a1b2c3d4") {
		t.Fatalf("txt %q", got[2].rdata)
	}
	if got[3].typ != typeA || !net.IP(got[3].rdata).Equal(ips[0]) {
		t.Fatalf("a %v", got[3].rdata)
	}
	// Not for us.
	if r, _ := Respond(query("_other._tcp.local.", typePTR), svc, ips, 120); r != nil {
		t.Fatal("answered foreign query")
	}
	// Host lookup.
	if r, _ := Respond(query("telecyborg-desktop-42.local.", typeA), svc, ips, 120); len(parseAnswers(t, r)) != 1 {
		t.Fatal("host A")
	}
}

func TestHostLabel(t *testing.T) {
	if got := HostLabel("José's PC_01"); got != "telecyborg-jos-s-pc-01" {
		t.Fatal(got)
	}
}
