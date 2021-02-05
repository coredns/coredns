package transfer

import (
	"context"
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

type (
	t1 struct{}
	t2 struct{}
)

func (t t1) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	const z = "example.org."
	if zone != z {
		return nil, ErrNotAuthoritative
	}
	return nil, fmt.Errorf(z)
}
func (t t2) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	const z = "sub.example.org."
	if zone != z {
		return nil, ErrNotAuthoritative
	}
	return nil, fmt.Errorf(z)
}

func TestZoneSelection(t *testing.T) {
	tr := &Transfer{
		Transferers: []Transferer{t1{}, t2{}},
		xfrs: []*xfr{
			{
				Zones: []string{"example.org."},
				to:    []string{"192.0.2.1"}, // RFC 5737 IP, no interface should have this address.
			},
			{
				Zones: []string{"sub.example.org."},
				to:    []string{"*"},
			},
		},
	}
	r := new(dns.Msg)
	r.SetAxfr("sub.example.org.")
	w := dnstest.NewRecorder(&test.ResponseWriter{TCP: true})
	_, err := tr.ServeDNS(context.TODO(), w, r)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if x := err.Error(); x != "sub.example.org." {
		t.Errorf("Expected transfer for zone %s, got %s", "sub.example.org", x)
	}
}
