package secondary

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/fall"
	plugintest "github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestTransferInCatalog(t *testing.T) {
	const origin = "catalog.example."
	zones := newTestTransferZones(map[string][]dns.RR{
		origin:         catalogZoneRecords(t, true),
		"example.org.": memberZoneRecords(t),
	})

	server := dnstest.NewServer(zones.handler())
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := newTestSecondary(origin, z, true)
	t.Cleanup(s.stopDynamicZones)

	if err := s.transferIn(origin, z, nil); err != nil {
		t.Fatalf("transferIn returned error: %v", err)
	}

	s.catalogMu.RLock()
	cat := s.catalogs[origin]
	s.catalogMu.RUnlock()
	if cat == nil {
		t.Fatal("expected parsed catalog")
	}
	if len(cat.Members) != 1 {
		t.Fatalf("expected 1 catalog member, got %d", len(cat.Members))
	}
	if cat.Members[0].Zone != "example.org." {
		t.Fatalf("expected member zone example.org., got %q", cat.Members[0].Zone)
	}
	if strings.Join(cat.Members[0].Groups, ",") != "default" {
		t.Fatalf("expected default member group, got %v", cat.Members[0].Groups)
	}
	if z.SOA == nil {
		t.Fatal("expected zone data to be live after valid catalog transfer")
	}

	msg := waitForAnswer(t, s, "www.example.org.", dns.TypeA)
	if len(msg.Answer) != 1 {
		t.Fatalf("expected 1 member zone answer, got %d", len(msg.Answer))
	}
	a, ok := msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A answer, got %T", msg.Answer[0])
	}
	if a.A.String() != "192.0.2.1" {
		t.Fatalf("expected 192.0.2.1, got %s", a.A.String())
	}
}

func TestTransferInCatalogRemovesMemberZone(t *testing.T) {
	const origin = "catalog.example."
	zones := newTestTransferZones(map[string][]dns.RR{
		origin:         catalogZoneRecords(t, true),
		"example.org.": memberZoneRecords(t),
	})

	server := dnstest.NewServer(zones.handler())
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := newTestSecondary(origin, z, true)
	t.Cleanup(s.stopDynamicZones)

	if err := s.transferIn(origin, z, nil); err != nil {
		t.Fatalf("transferIn returned error: %v", err)
	}
	waitForAnswer(t, s, "www.example.org.", dns.TypeA)

	zones.set(origin, catalogZoneRecordsWithoutMembers(t))
	if err := s.transferIn(origin, z, nil); err != nil {
		t.Fatalf("transferIn returned error: %v", err)
	}
	if _, _, ok := s.lookupZone("www.example.org."); ok {
		t.Fatal("expected member zone to be removed after catalog update")
	}
}

func TestTransferInCatalogRejectsInvalidCatalog(t *testing.T) {
	const origin = "catalog.example."
	rrs := catalogZoneRecords(t, false)

	server := dnstest.NewServer(newTestTransferZones(map[string][]dns.RR{origin: rrs}).handler())
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := newTestSecondary(origin, z, true)

	err := s.transferIn(origin, z, nil)
	if err == nil {
		t.Fatal("expected invalid catalog transfer to fail")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected version error, got %v", err)
	}
	if s.catalogs[origin] != nil {
		t.Fatal("expected invalid catalog not to be stored")
	}
	if z.SOA != nil {
		t.Fatal("expected invalid catalog transfer not to replace live zone data")
	}
}

func TestTransferInSkipsCatalogParseForRegularZone(t *testing.T) {
	const origin = "catalog.example."
	rrs := catalogZoneRecords(t, false)

	server := dnstest.NewServer(newTestTransferZones(map[string][]dns.RR{origin: rrs}).handler())
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := newTestSecondary(origin, z, false)

	if err := s.transferIn(origin, z, nil); err != nil {
		t.Fatalf("transferIn returned error: %v", err)
	}
	if s.catalogs[origin] != nil {
		t.Fatal("expected regular secondary transfer not to store catalog")
	}
	if z.SOA == nil {
		t.Fatal("expected regular secondary transfer to replace live zone data")
	}
}

type testTransferZones struct {
	mu      sync.RWMutex
	records map[string][]dns.RR
}

func newTestTransferZones(records map[string][]dns.RR) *testTransferZones {
	return &testTransferZones{records: records}
}

func (z *testTransferZones) set(zone string, rrs []dns.RR) {
	z.mu.Lock()
	z.records[zone] = rrs
	z.mu.Unlock()
}

func (z *testTransferZones) handler() dns.HandlerFunc {
	return func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(req)
		if len(req.Question) > 0 {
			qname := strings.ToLower(dns.Fqdn(req.Question[0].Name))
			z.mu.RLock()
			rrs := append([]dns.RR(nil), z.records[qname]...)
			z.mu.RUnlock()

			switch req.Question[0].Qtype {
			case dns.TypeSOA:
				for _, rr := range rrs {
					if rr.Header().Rrtype == dns.TypeSOA {
						m.Answer = []dns.RR{rr}
						break
					}
				}
			case dns.TypeAXFR:
				m.Answer = rrs
			}
		}
		_ = w.WriteMsg(m)
	}
}

func newTestSecondary(origin string, z *file.Zone, catalog bool) *Secondary {
	catalogZones := map[string]struct{}{}
	if catalog {
		catalogZones[origin] = struct{}{}
	}
	return newSecondary(file.Zones{Z: map[string]*file.Zone{origin: z}, Names: []string{origin}}, fall.F{}, catalogZones)
}

func waitForAnswer(t *testing.T, s *Secondary, name string, qtype uint16) *dns.Msg {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		req := new(dns.Msg)
		req.SetQuestion(name, qtype)
		rec := dnstest.NewRecorder(&plugintest.ResponseWriter{})
		code, err := s.ServeDNS(context.Background(), rec, req)
		if err == nil && code == dns.RcodeSuccess && rec.Msg != nil && rec.Msg.Rcode == dns.RcodeSuccess && len(rec.Msg.Answer) > 0 {
			return rec.Msg
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for answer to %s", name)
	return nil
}

func catalogZoneRecords(t *testing.T, includeVersion bool) []dns.RR {
	t.Helper()

	soa := mustRR(t, "catalog.example. 0 IN SOA invalid. hostmaster.invalid. 1 3600 600 604800 0")
	rrs := []dns.RR{
		soa,
		mustRR(t, "catalog.example. 0 IN NS invalid."),
	}
	if includeVersion {
		rrs = append(rrs, mustRR(t, `version.catalog.example. 0 IN TXT "2"`))
	}
	rrs = append(rrs,
		mustRR(t, "a.zones.catalog.example. 0 IN PTR example.org."),
		mustRR(t, `group.a.zones.catalog.example. 0 IN TXT "default"`),
		soa,
	)
	return rrs
}

func catalogZoneRecordsWithoutMembers(t *testing.T) []dns.RR {
	t.Helper()

	soa := mustRR(t, "catalog.example. 0 IN SOA invalid. hostmaster.invalid. 2 3600 600 604800 0")
	return []dns.RR{
		soa,
		mustRR(t, "catalog.example. 0 IN NS invalid."),
		mustRR(t, `version.catalog.example. 0 IN TXT "2"`),
		soa,
	}
}

func memberZoneRecords(t *testing.T) []dns.RR {
	t.Helper()

	soa := mustRR(t, "example.org. 0 IN SOA ns.example.org. hostmaster.example.org. 1 3600 600 604800 0")
	return []dns.RR{
		soa,
		mustRR(t, "example.org. 0 IN NS ns.example.org."),
		mustRR(t, "www.example.org. 0 IN A 192.0.2.1"),
		soa,
	}
}

func mustRR(t *testing.T, s string) dns.RR {
	t.Helper()

	rr, err := dns.NewRR(s)
	if err != nil {
		t.Fatalf("failed to parse RR %q: %v", s, err)
	}
	return rr
}
