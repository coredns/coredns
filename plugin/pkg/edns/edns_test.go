package edns

import (
	"testing"

	"github.com/miekg/dns"
)

func TestVersion(t *testing.T) {
	m := ednsMsg()
	m.Extra[0].(*dns.OPT).SetVersion(2)

	r, err := Version(m)
	if err == nil {
		t.Errorf("Expected wrong version, but got OK")
	}
	if r.Question == nil {
		t.Errorf("Expected question section, but got nil")
	}
	if r.Rcode != dns.RcodeBadVers {
		t.Errorf("Expected Rcode to be of BADVER (16), but got %d", r.Rcode)
	}
	if r.Extra == nil {
		t.Errorf("Expected OPT section, but got nil")
	}
}

func TestVersionNoEdns(t *testing.T) {
	m := ednsMsg()
	m.Extra = nil

	r, err := Version(m)
	if err != nil {
		t.Errorf("Expected no error, but got one: %s", err)
	}
	if r != nil {
		t.Errorf("Expected nil since not an EDNS0 request, but did not got nil")
	}
}

func ednsMsg() *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	o := new(dns.OPT)
	o.Hdr.Name = "."
	o.Hdr.Rrtype = dns.TypeOPT
	m.Extra = append(m.Extra, o)
	return m
}
