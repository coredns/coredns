package file

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// TestLookupReloadRace exercises the query-time helpers that resolve in-zone
// CNAME chains (externalLookup), additional-section targets
// (additionalProcessing) and the DNSSEC closest encloser (ClosestEncloser)
// concurrently with a reload/transfer that swaps the zone's Apex and tree via
// setData. Before the fix these helpers read z.Tree/z.Apex directly instead of
// the generation pinned by Lookup's snapshot, racing the swap. Run with -race.
func TestLookupReloadRace(t *testing.T) {
	zone, err := Parse(strings.NewReader(dbMiekNL), testzone, "stdin", 0)
	if err != nil {
		t.Fatalf("Expected no error when reading zone, got %q", err)
	}

	// Pre-parse a few generations to swap in from the writer goroutine.
	gens := make([]*Zone, 4)
	for i := range gens {
		z, err := Parse(strings.NewReader(dbMiekNL), testzone, "stdin", 0)
		if err != nil {
			t.Fatalf("Expected no error when reading zone, got %q", err)
		}
		gens[i] = z
	}

	ctx := context.TODO()
	// Names that reach each helper: an in-zone CNAME, an SRV/MX with an in-zone
	// target, and a non-existent name (with DO set) for the NXDOMAIN path.
	queries := []struct {
		qname string
		qtype uint16
		do    bool
	}{
		{"www.miek.nl.", dns.TypeA, false},   // externalLookup
		{"srv.miek.nl.", dns.TypeSRV, false}, // additionalProcessing
		{"mx.miek.nl.", dns.TypeMX, false},   // additionalProcessing
		{"nx.miek.nl.", dns.TypeA, true},     // ClosestEncloser (NXDOMAIN + DO)
	}

	stop := make(chan struct{})

	// Writer: keep swapping the zone's data generation until told to stop.
	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			g := gens[i%len(gens)]
			zone.setData(g.Apex, g.Tree)
			i++
		}
	}()

	// Readers: hammer the lookup paths that use the helpers.
	var readers sync.WaitGroup
	for _, q := range queries {
		readers.Add(1)
		go func(qname string, qtype uint16, do bool) {
			defer readers.Done()
			m := new(dns.Msg)
			m.SetQuestion(qname, qtype)
			m.SetEdns0(4096, do)
			state := request.Request{W: &test.ResponseWriter{}, Req: m}
			for j := 0; j < 3000; j++ {
				zone.Lookup(ctx, state, qname)
			}
		}(q.qname, q.qtype, q.do)
	}

	readers.Wait()
	close(stop)
	writer.Wait()
}
