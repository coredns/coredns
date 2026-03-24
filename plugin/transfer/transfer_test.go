package transfer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// transfererPlugin implements transfer.Transferer and plugin.Handler.
type transfererPlugin struct {
	Zone   string
	Serial uint32
	Next   plugin.Handler
}

// Name implements plugin.Handler.
func (*transfererPlugin) Name() string { return "transfererplugin" }

// ServeDNS implements plugin.Handler.
func (p *transfererPlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if r.Question[0].Name != p.Zone {
		return p.Next.ServeDNS(ctx, w, r)
	}
	return 0, nil
}

// Transfer implements transfer.Transferer - it returns a static AXFR response, or
// if serial is current, an abbreviated IXFR response.
func (p *transfererPlugin) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	if zone != p.Zone {
		return nil, ErrNotAuthoritative
	}
	ch := make(chan []dns.RR, 3) // sending 3 bits and don't want to block, nor do a waitgroup
	defer close(ch)
	ch <- []dns.RR{test.SOA(fmt.Sprintf("%s 100 IN SOA ns.dns.%s hostmaster.%s %d 7200 1800 86400 100", p.Zone, p.Zone, p.Zone, p.Serial))}
	if serial >= p.Serial {
		return ch, nil
	}
	ch <- []dns.RR{
		test.NS(fmt.Sprintf("%s 100 IN NS ns.dns.%s", p.Zone, p.Zone)),
		test.A(fmt.Sprintf("ns.dns.%s 100 IN A 1.2.3.4", p.Zone)),
	}
	ch <- []dns.RR{test.SOA(fmt.Sprintf("%s 100 IN SOA ns.dns.%s hostmaster.%s %d 7200 1800 86400 100", p.Zone, p.Zone, p.Zone, p.Serial))}
	return ch, nil
}

type terminatingPlugin struct{}

// Name implements plugin.Handler.
func (*terminatingPlugin) Name() string { return "testplugin" }

// ServeDNS implements plugin.Handler that returns NXDOMAIN for all requests.
func (*terminatingPlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	m := new(dns.Msg)
	m.SetRcode(r, dns.RcodeNameError)
	w.WriteMsg(m)
	return dns.RcodeNameError, nil
}

func newTestTransfer() *Transfer {
	nextPlugin1 := transfererPlugin{Zone: "example.com.", Serial: 12345}
	nextPlugin2 := transfererPlugin{Zone: "example.org.", Serial: 12345}
	nextPlugin2.Next = &terminatingPlugin{}
	nextPlugin1.Next = &nextPlugin2

	transfer := &Transfer{
		Transferers: []Transferer{&nextPlugin1, &nextPlugin2},
		xfrs: []*xfr{
			{
				Zones: []string{"example.org."},
				to:    []string{"*"},
			},
			{
				Zones: []string{"example.com."},
				to:    []string{"*"},
			},
		},
		Next: &nextPlugin1,
	}
	return transfer
}

func TestTransferNonZone(t *testing.T) {
	transfer := newTestTransfer()
	ctx := context.TODO()

	for _, tc := range []string{"sub.example.org.", "example.test."} {
		w := dnstest.NewRecorder(&test.ResponseWriter{TCP: true})
		m := &dns.Msg{}
		m.SetAxfr(tc)

		_, err := transfer.ServeDNS(ctx, w, m)
		if err != nil {
			t.Error(err)
		}

		if w.Msg == nil {
			t.Fatalf("Got nil message for AXFR %s", tc)
		}

		if w.Msg.Rcode != dns.RcodeNameError {
			t.Errorf("Expected NXDOMAIN for AXFR %s got %s", tc, dns.RcodeToString[w.Msg.Rcode])
		}
	}
}

func TestTransferNotAXFRorIXFR(t *testing.T) {
	transfer := newTestTransfer()

	ctx := context.TODO()
	w := dnstest.NewRecorder(&test.ResponseWriter{TCP: true})
	m := &dns.Msg{}
	m.SetQuestion("test.domain.", dns.TypeA)

	_, err := transfer.ServeDNS(ctx, w, m)
	if err != nil {
		t.Error(err)
	}

	if w.Msg == nil {
		t.Fatal("Got nil message")
	}

	if w.Msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN got %s", dns.RcodeToString[w.Msg.Rcode])
	}
}

func TestTransferAXFRExampleOrg(t *testing.T) {
	transfer := newTestTransfer()

	ctx := context.TODO()
	w := dnstest.NewMultiRecorder(&test.ResponseWriter{TCP: true})
	m := &dns.Msg{}
	m.SetAxfr(transfer.xfrs[0].Zones[0])

	_, err := transfer.ServeDNS(ctx, w, m)
	if err != nil {
		t.Error(err)
	}

	validateAXFRResponse(t, w)
}

func TestTransferAXFRExampleCom(t *testing.T) {
	transfer := newTestTransfer()

	ctx := context.TODO()
	w := dnstest.NewMultiRecorder(&test.ResponseWriter{TCP: true})
	m := &dns.Msg{}
	m.SetAxfr(transfer.xfrs[1].Zones[0])

	_, err := transfer.ServeDNS(ctx, w, m)
	if err != nil {
		t.Error(err)
	}

	validateAXFRResponse(t, w)
}

func TestTransferIXFRCurrent(t *testing.T) {
	transfer := newTestTransfer()

	testPlugin := transfer.Transferers[0].(*transfererPlugin)

	ctx := context.TODO()
	w := dnstest.NewMultiRecorder(&test.ResponseWriter{TCP: true})
	m := &dns.Msg{}
	m.SetIxfr(transfer.xfrs[0].Zones[0], testPlugin.Serial, "ns.dns."+testPlugin.Zone, "hostmaster.dns."+testPlugin.Zone)

	_, err := transfer.ServeDNS(ctx, w, m)
	if err != nil {
		t.Error(err)
	}

	if len(w.Msgs) == 0 {
		t.Fatal("Did not get back a zone response")
	}

	if len(w.Msgs[0].Answer) != 1 {
		t.Logf("%+v\n", w)
		t.Fatalf("Expected 1 answer, got %d", len(w.Msgs[0].Answer))
	}

	// Ensure the answer is the SOA
	if w.Msgs[0].Answer[0].Header().Rrtype != dns.TypeSOA {
		t.Error("Answer does not contain the SOA record")
	}
}

func TestTransferIXFRFallback(t *testing.T) {
	transfer := newTestTransfer()

	testPlugin := transfer.Transferers[0].(*transfererPlugin)

	ctx := context.TODO()
	w := dnstest.NewMultiRecorder(&test.ResponseWriter{TCP: true})
	m := &dns.Msg{}
	m.SetIxfr(
		transfer.xfrs[0].Zones[0],
		testPlugin.Serial-1,
		"ns.dns."+testPlugin.Zone,
		"hostmaster.dns."+testPlugin.Zone,
	)

	_, err := transfer.ServeDNS(ctx, w, m)
	if err != nil {
		t.Error(err)
	}

	validateAXFRResponse(t, w)
}

func validateAXFRResponse(t *testing.T, w *dnstest.MultiRecorder) {
	t.Helper()
	if len(w.Msgs) == 0 {
		t.Fatal("Did not get back a zone response")
	}

	if len(w.Msgs[0].Answer) == 0 {
		t.Logf("%+v\n", w)
		t.Fatal("Did not get back an answer")
	}

	// Ensure the answer starts with SOA
	if w.Msgs[0].Answer[0].Header().Rrtype != dns.TypeSOA {
		t.Error("Answer does not start with SOA record")
	}

	// Ensure the answer ends with SOA
	if w.Msgs[len(w.Msgs)-1].Answer[len(w.Msgs[len(w.Msgs)-1].Answer)-1].Header().Rrtype != dns.TypeSOA {
		t.Error("Answer does not end with SOA record")
	}

	// Ensure the answer is the expected length
	c := 0
	for _, m := range w.Msgs {
		c += len(m.Answer)
	}
	if c != 4 {
		t.Errorf("Answer is not the expected length (expected 4, got %d)", c)
	}
}

func TestTransferNotAllowed(t *testing.T) {
	nextPlugin := transfererPlugin{Zone: "example.org.", Serial: 12345}

	transfer := Transfer{
		Transferers: []Transferer{&nextPlugin},
		xfrs: []*xfr{
			{
				Zones: []string{"example.org."},
				to:    []string{"1.2.3.4"},
			},
		},
		Next: &nextPlugin,
	}

	ctx := context.TODO()
	w := dnstest.NewRecorder(&test.ResponseWriter{TCP: true})
	m := &dns.Msg{}
	m.SetAxfr(transfer.xfrs[0].Zones[0])

	_, err := transfer.ServeDNS(ctx, w, m)

	if err != nil {
		t.Error(err)
	}

	if w.Msg.Rcode != dns.RcodeRefused {
		t.Errorf("Expected REFUSED response code, got %s", dns.RcodeToString[w.Msg.Rcode])
	}
}

// errWriter is a dns.ResponseWriter that simulates a client error on write.
type errWriter struct {
	test.ResponseWriter
}

func (e *errWriter) WriteMsg(m *dns.Msg) error { return fmt.Errorf("write error") }

// blockingTransferer produces many records into the channel and signals when done.
type blockingTransferer struct {
	Zone string
	done chan struct{}
}

func (b *blockingTransferer) Name() string { return "blockingtransferer" }
func (b *blockingTransferer) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return 0, nil
}
func (b *blockingTransferer) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	if zone != b.Zone {
		return nil, ErrNotAuthoritative
	}
	ch := make(chan []dns.RR, 2)
	go func() {
		defer close(ch)
		defer close(b.done)
		soa := test.SOA(fmt.Sprintf("%s 100 IN SOA ns.dns.%s hostmaster.%s %d 7200 1800 86400 100", b.Zone, b.Zone, b.Zone, 1))
		ch <- []dns.RR{soa}
		for range 2000 {
			ch <- []dns.RR{test.A("ns.dns." + b.Zone + " 100 IN A 1.2.3.4")}
		}
		ch <- []dns.RR{soa}
	}()
	return ch, nil
}

// Test that when the client errors mid-transfer, the server drains the producer channel
// so the producer goroutine can complete (no leak/block on small buffer).
func TestTransferDrainsProducerOnClientError(t *testing.T) {
	b := &blockingTransferer{Zone: "example.org.", done: make(chan struct{})}

	transfer := &Transfer{
		Transferers: []Transferer{b},
		xfrs:        []*xfr{{Zones: []string{"example.org."}, to: []string{"*"}}},
		Next:        b,
	}

	ctx := context.TODO()
	w := &errWriter{ResponseWriter: test.ResponseWriter{TCP: true}}
	m := &dns.Msg{}
	m.SetAxfr("example.org.")

	_, _ = transfer.ServeDNS(ctx, w, m)

	select {
	case <-b.done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("producer goroutine did not finish; channel likely not drained on client error")
	}
}

type nopHandler struct{}

func (nopHandler) Name() string { return "nop" }
func (nopHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return dns.RcodeSuccess, nil
}

func TestAXFRZoneMatchCaseInsensitive(t *testing.T) {
	next := &transfererPlugin{Zone: "example.org.", Serial: 12345}
	next.Next = nopHandler{}

	tr := &Transfer{
		Transferers: []Transferer{next},
		xfrs:        []*xfr{{Zones: []string{"example.org."}, to: []string{"*"}}},
		Next:        next,
	}

	ctx := context.TODO()
	w := dnstest.NewMultiRecorder(&test.ResponseWriter{TCP: true})
	m := new(dns.Msg)
	m.SetAxfr("ExAmPlE.OrG.") // mixed case

	_, err := tr.ServeDNS(ctx, w, m)
	if err != nil {
		t.Fatalf("ServeDNS error: %v", err)
	}

	validateAXFRResponse(t, w)
}

func TestLongestMatchMostSpecificZone(t *testing.T) {
	x1 := &xfr{Zones: []string{"example.org."}}
	x2 := &xfr{Zones: []string{"a.example.org."}}

	got := longestMatch([]*xfr{x1, x2}, "host.a.example.org.")
	if got != x2 {
		t.Fatalf("expected most specific zone (a.example.org.) to match, got %+v", got)
	}
}

func TestLongestMatchNilWhenNoMatch(t *testing.T) {
	x1 := &xfr{Zones: []string{"example.org."}}

	got := longestMatch([]*xfr{x1}, "other.net.")
	if got != nil {
		t.Fatalf("expected nil when no zones match, got %+v", got)
	}
}
