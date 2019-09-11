package transfer

import (
	"context"
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

type testPlugin struct {
	Zone   string
	Serial uint32
	Next   plugin.Handler
}

func (p testPlugin) Name() string { return "testplugin" }

func (p testPlugin) ServeDNS(context.Context, dns.ResponseWriter, *dns.Msg) (int, error) {
	return 0, nil
}

func (p testPlugin) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	ch := make(chan []dns.RR, 2)
	defer close(ch)
	ch <- []dns.RR{test.SOA(fmt.Sprintf("%s 100 IN SOA ns.dns.%s hostmaster.%s %d 7200 1800 86400 100", p.Zone, p.Zone, p.Zone, p.Serial))}
	if serial >= p.Serial {
		return ch, nil
	}
	ch <- []dns.RR{
		test.NS(fmt.Sprintf("%s 100 IN NS ns.dns.%s", p.Zone, p.Zone)),
		test.A(fmt.Sprintf("ns.dns.%s 100 IN A 1.2.3.4", p.Zone)),
	}
	return ch, nil
}

func newTestTransfer() Transfer {
	nextPlugin1 := testPlugin{Zone: "example.com.", Serial: 12345}
	nextPlugin2 := testPlugin{Zone: "example.org.", Serial: 12345}
	nextPlugin1.Next = nextPlugin2

	transfer := Transfer{
		Next: nextPlugin1,
		xfrs: []*xfr{
			{
				Zones:       []string{"example.org."},
				to:          []string{"*"},
				Transferers: []Transferer{nextPlugin1, nextPlugin2},
			},
			{
				Zones:       []string{"example.com."},
				to:          []string{"*"},
				Transferers: []Transferer{nextPlugin1, nextPlugin2},
			},
		},
	}
	return transfer
}

func TestTransferAXFRExampleOrg(t *testing.T) {

	transfer := newTestTransfer()

	ctx := context.TODO()
	w := dnstest.NewMultiRecorder(&test.ResponseWriter{})
	dnsmsg := &dns.Msg{}
	dnsmsg.SetAxfr(transfer.xfrs[0].Zones[0])

	_, err := transfer.ServeDNS(ctx, w, dnsmsg)
	if err != nil {
		t.Error(err)
	}

	validateAXFRResponse(t, w)
}
func TestTransferAXFRExampleCom(t *testing.T) {

	transfer := newTestTransfer()

	ctx := context.TODO()
	w := dnstest.NewMultiRecorder(&test.ResponseWriter{})
	dnsmsg := &dns.Msg{}
	dnsmsg.SetAxfr(transfer.xfrs[1].Zones[0])

	_, err := transfer.ServeDNS(ctx, w, dnsmsg)
	if err != nil {
		t.Error(err)
	}

	validateAXFRResponse(t, w)
}

func TestTransferIXFRFallback(t *testing.T) {

	transfer := newTestTransfer()

	testPlugin := transfer.xfrs[0].Transferers[0].(testPlugin)

	ctx := context.TODO()
	w := dnstest.NewMultiRecorder(&test.ResponseWriter{})
	dnsmsg := &dns.Msg{}
	dnsmsg.SetIxfr(
		transfer.xfrs[0].Zones[0],
		testPlugin.Serial-1,
		"ns.dns."+testPlugin.Zone,
		"hostmaster.dns."+testPlugin.Zone,
	)

	_, err := transfer.ServeDNS(ctx, w, dnsmsg)
	if err != nil {
		t.Error(err)
	}

	validateAXFRResponse(t, w)
}

func TestTransferIXFRCurrent(t *testing.T) {

	transfer := newTestTransfer()

	testPlugin := transfer.xfrs[0].Transferers[0].(testPlugin)

	ctx := context.TODO()
	w := dnstest.NewMultiRecorder(&test.ResponseWriter{})
	dnsmsg := &dns.Msg{}
	dnsmsg.SetIxfr(
		transfer.xfrs[0].Zones[0],
		testPlugin.Serial,
		"ns.dns."+testPlugin.Zone,
		"hostmaster.dns."+testPlugin.Zone,
	)

	_, err := transfer.ServeDNS(ctx, w, dnsmsg)
	if err != nil {
		t.Error(err)
	}

	if len(w.Msgs) == 0 {
		t.Logf("%+v\n", w)
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

func validateAXFRResponse(t *testing.T, w *dnstest.MultiRecorder) {
	if len(w.Msgs) == 0 {
		t.Logf("%+v\n", w)
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
	nextPlugin := testPlugin{Zone: "example.org.", Serial: 12345}

	transfer := Transfer{
		Next: nextPlugin,
		xfrs: []*xfr{
			{
				Zones:       []string{"example.org."},
				to:          []string{"1.2.3.4"},
				Transferers: []Transferer{nextPlugin},
			},
		},
	}

	ctx := context.TODO()
	w := dnstest.NewMultiRecorder(&test.ResponseWriter{})
	dnsmsg := &dns.Msg{}
	dnsmsg.SetAxfr(transfer.xfrs[0].Zones[0])

	rcode, err := transfer.ServeDNS(ctx, w, dnsmsg)

	if err != nil {
		t.Error(err)
	}

	if rcode != dns.RcodeRefused {
		t.Errorf("Expected REFUSED response code, got %s", dns.RcodeToString[rcode])
	}

}

// difference shows what we're missing when comparing two RR slices
func difference(testRRs []dns.RR, gotRRs []dns.RR) []dns.RR {
	expectedRRs := map[string]struct{}{}
	for _, rr := range testRRs {
		expectedRRs[rr.String()] = struct{}{}
	}

	foundRRs := []dns.RR{}
	for _, rr := range gotRRs {
		if _, ok := expectedRRs[rr.String()]; !ok {
			foundRRs = append(foundRRs, rr)
		}
	}
	return foundRRs
}
