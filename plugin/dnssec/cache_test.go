package dnssec

import (
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/cache"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestCacheSet(t *testing.T) {
	fPriv, rmPriv, _ := test.TempFile(".", privKey)
	fPub, rmPub, _ := test.TempFile(".", pubKey)
	defer rmPriv()
	defer rmPub()

	dnskey, err := ParseKeyFile(fPub, fPriv)
	if err != nil {
		t.Fatalf("Failed to parse key: %v\n", err)
	}

	c := cache.New[[]dns.RR](defaultCap)
	m := testMsg()
	state := request.Request{Req: m, Zone: "miek.nl."}
	k := hash(m.Answer) // calculate *before* we add the sig
	d := New([]string{"miek.nl."}, []*DNSKEY{dnskey}, false, nil, c)
	d.Sign(state, time.Now().UTC(), server)

	_, ok := d.get(k, server)
	if !ok {
		t.Errorf("Signature was not added to the cache")
	}
}

func TestCacheNotValidExpired(t *testing.T) {
	fPriv, rmPriv, _ := test.TempFile(".", privKey)
	fPub, rmPub, _ := test.TempFile(".", pubKey)
	defer rmPriv()
	defer rmPub()

	dnskey, err := ParseKeyFile(fPub, fPriv)
	if err != nil {
		t.Fatalf("Failed to parse key: %v\n", err)
	}

	c := cache.New[[]dns.RR](defaultCap)
	m := testMsg()
	state := request.Request{Req: m, Zone: "miek.nl."}
	k := hash(m.Answer) // calculate *before* we add the sig
	d := New([]string{"miek.nl."}, []*DNSKEY{dnskey}, false, nil, c)
	d.Sign(state, time.Now().UTC().AddDate(0, 0, -9), server)

	_, ok := d.get(k, server)
	if ok {
		t.Errorf("Signature was added to the cache even though not valid")
	}
}

func TestCacheEmptySigsNotCached(t *testing.T) {
	c := cache.New[[]dns.RR](defaultCap)
	m := testMsg()
	state := request.Request{Req: m, Zone: "miek.nl."}
	k := hash(m.Answer)

	// Create a Dnssec instance with no keys; sign() will produce no signatures.
	d := New([]string{"miek.nl."}, []*DNSKEY{}, false, nil, c)
	d.Sign(state, time.Now().UTC(), server)

	_, ok := d.get(k, server)
	if ok {
		t.Errorf("Empty signatures should not be cached")
	}
}

func TestCacheNotValidYet(t *testing.T) {
	fPriv, rmPriv, _ := test.TempFile(".", privKey)
	fPub, rmPub, _ := test.TempFile(".", pubKey)
	defer rmPriv()
	defer rmPub()

	dnskey, err := ParseKeyFile(fPub, fPriv)
	if err != nil {
		t.Fatalf("Failed to parse key: %v\n", err)
	}

	c := cache.New[[]dns.RR](defaultCap)
	m := testMsg()
	state := request.Request{Req: m, Zone: "miek.nl."}
	k := hash(m.Answer) // calculate *before* we add the sig
	d := New([]string{"miek.nl."}, []*DNSKEY{dnskey}, false, nil, c)
	d.Sign(state, time.Now().UTC().AddDate(0, 0, +9), server)

	_, ok := d.get(k, server)
	if ok {
		t.Errorf("Signature was added to the cache even though not valid yet")
	}
}
