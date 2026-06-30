package secondary

import (
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/catalog"
	"github.com/coredns/coredns/plugin/pkg/dnstest"

	"github.com/miekg/dns"
)

func TestTransferInCatalog(t *testing.T) {
	const origin = "catalog.example."
	rrs := catalogZoneRecords(t, true)

	server := dnstest.NewServer(transferHandler(rrs))
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := &Secondary{
		catalogs:     make(map[string]*catalog.Catalog),
		catalogZones: map[string]struct{}{origin: {}},
	}

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
}

func TestTransferInCatalogRejectsInvalidCatalog(t *testing.T) {
	const origin = "catalog.example."
	rrs := catalogZoneRecords(t, false)

	server := dnstest.NewServer(transferHandler(rrs))
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := &Secondary{
		catalogs:     make(map[string]*catalog.Catalog),
		catalogZones: map[string]struct{}{origin: {}},
	}

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

	server := dnstest.NewServer(transferHandler(rrs))
	defer server.Close()

	z := file.NewZone(origin, "stdin")
	z.TransferFrom = []string{server.Addr}
	s := &Secondary{
		catalogs: make(map[string]*catalog.Catalog),
	}

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

func transferHandler(rrs []dns.RR) dns.HandlerFunc {
	return func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(req)
		if len(req.Question) > 0 {
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

func mustRR(t *testing.T, s string) dns.RR {
	t.Helper()

	rr, err := dns.NewRR(s)
	if err != nil {
		t.Fatalf("failed to parse RR %q: %v", s, err)
	}
	return rr
}
