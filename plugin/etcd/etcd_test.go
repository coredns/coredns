package etcd

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	"go.etcd.io/etcd/api/v3/mvccpb"
	etcdcv3 "go.etcd.io/etcd/client/v3"
)

type memoryKV struct {
	etcdcv3.KV
	data map[string]string
}

func (m *memoryKV) Get(_ context.Context, key string, opts ...etcdcv3.OpOption) (*etcdcv3.GetResponse, error) {
	op := etcdcv3.OpGet(key, opts...)
	var keys []string
	if op.IsOptsWithPrefix() {
		for k := range m.data {
			if strings.HasPrefix(k, key) {
				keys = append(keys, k)
			}
		}
	} else if _, ok := m.data[key]; ok {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	kvs := make([]*mvccpb.KeyValue, 0, len(keys))
	for _, k := range keys {
		kvs = append(kvs, &mvccpb.KeyValue{
			Key:   []byte(k),
			Value: []byte(m.data[k]),
		})
	}
	return &etcdcv3.GetResponse{
		Kvs:   kvs,
		Count: int64(len(kvs)),
	}, nil
}

func TestServeDNSCNAMEAndIPWithText(t *testing.T) {
	const externalDNSText = `"heritage=external-dns,external-dns/owner=staging,external-dns/resource=ingress/stg-infra/goh.aminm.com-k8s"`

	kv := &memoryKV{
		data: map[string]string{
			"/skydns/com/aminm/goh/149a1668": `{"host":"target.aminm.com","text":"\"heritage=external-dns,external-dns/owner=staging,external-dns/resource=ingress/stg-infra/goh.aminm.com-k8s\"","targetstrip":1}`,
			"/skydns/com/aminm/exactcname":   `{"host":"target.aminm.com","text":"heritage=external-dns"}`,
			"/skydns/com/aminm/target/a":     `{"host":"10.0.0.1","text":"v=spf1"}`,
			"/skydns/com/aminm/target/aaaa":  `{"host":"2001:db8::1","text":"v=spf1-v6"}`,
			"/skydns/com/aminm/onlytxt":      `{"text":"just-txt"}`,
			"/skydns/com/aminm/txtalias":     `{"host":"onlytxt.aminm.com"}`,
		},
	}

	e := &Etcd{
		Zones:      []string{"aminm.com."},
		PathPrefix: "skydns",
		Client:     &etcdcv3.Client{KV: kv},
	}

	query := func(t *testing.T, qname string, qtype uint16) *dns.Msg {
		t.Helper()
		m := new(dns.Msg)
		m.SetQuestion(qname, qtype)
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		if _, err := e.ServeDNS(context.Background(), rec, m); err != nil {
			t.Fatalf("ServeDNS(%s, %s) returned error: %v", qname, dns.TypeToString[qtype], err)
		}
		return rec.Msg
	}

	checkExact := func(t *testing.T, resp *dns.Msg, tc test.Case) {
		t.Helper()
		if err := test.Header(tc, resp); err != nil {
			t.Fatal(err)
		}
		if err := test.Section(tc, test.Answer, resp.Answer); err != nil {
			t.Fatal(err)
		}
		if err := test.Section(tc, test.Ns, resp.Ns); err != nil {
			t.Fatal(err)
		}
		if err := test.Section(tc, test.Extra, resp.Extra); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("CNAME with text resolves A", func(t *testing.T) {
		resp := query(t, "goh.aminm.com.", dns.TypeA)
		checkExact(t, resp, test.Case{
			Qname: "goh.aminm.com.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.CNAME("goh.aminm.com. 300 IN CNAME target.aminm.com."),
				test.A("target.aminm.com. 300 IN A 10.0.0.1"),
			},
		})
	})

	t.Run("CNAME with text resolves AAAA", func(t *testing.T) {
		resp := query(t, "goh.aminm.com.", dns.TypeAAAA)
		checkExact(t, resp, test.Case{
			Qname: "goh.aminm.com.", Qtype: dns.TypeAAAA,
			Answer: []dns.RR{
				test.CNAME("goh.aminm.com. 300 IN CNAME target.aminm.com."),
				test.AAAA("target.aminm.com. 300 IN AAAA 2001:db8::1"),
			},
		})
	})

	t.Run("CNAME with text resolves CNAME", func(t *testing.T) {
		resp := query(t, "exactcname.aminm.com.", dns.TypeCNAME)
		checkExact(t, resp, test.Case{
			Qname: "exactcname.aminm.com.", Qtype: dns.TypeCNAME,
			Answer: []dns.RR{
				test.CNAME("exactcname.aminm.com. 300 IN CNAME target.aminm.com."),
			},
		})
	})

	t.Run("CNAME with text preserves TXT lookup", func(t *testing.T) {
		resp := query(t, "goh.aminm.com.", dns.TypeTXT)
		if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
			t.Fatalf("expected 1 TXT answer, got rcode=%s answer=%v", dns.RcodeToString[resp.Rcode], resp.Answer)
		}
		txtRec, ok := resp.Answer[0].(*dns.TXT)
		if !ok {
			t.Fatalf("expected TXT RR, got %T", resp.Answer[0])
		}
		if got := strings.Join(txtRec.Txt, ""); got != externalDNSText {
			t.Fatalf("expected TXT %q, got %q", externalDNSText, got)
		}
	})

	t.Run("IPv4 and IPv6 with text serve A, AAAA, and TXT", func(t *testing.T) {
		respA := query(t, "target.aminm.com.", dns.TypeA)
		if err := test.SortAndCheck(respA, test.Case{
			Qname: "target.aminm.com.", Qtype: dns.TypeA,
			Answer: []dns.RR{test.A("target.aminm.com. 300 IN A 10.0.0.1")},
		}); err != nil {
			t.Fatal(err)
		}

		respAAAA := query(t, "target.aminm.com.", dns.TypeAAAA)
		if err := test.SortAndCheck(respAAAA, test.Case{
			Qname: "target.aminm.com.", Qtype: dns.TypeAAAA,
			Answer: []dns.RR{test.AAAA("target.aminm.com. 300 IN AAAA 2001:db8::1")},
		}); err != nil {
			t.Fatal(err)
		}

		respTXT := query(t, "target.aminm.com.", dns.TypeTXT)
		if err := test.SortAndCheck(respTXT, test.Case{
			Qname: "target.aminm.com.", Qtype: dns.TypeTXT,
			Answer: []dns.RR{
				test.TXT(`target.aminm.com. 300 IN TXT "v=spf1"`),
				test.TXT(`target.aminm.com. 300 IN TXT "v=spf1-v6"`),
			},
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("pure TXT serves TXT and does not serve CNAME", func(t *testing.T) {
		respTXT := query(t, "onlytxt.aminm.com.", dns.TypeTXT)
		if err := test.SortAndCheck(respTXT, test.Case{
			Qname: "onlytxt.aminm.com.", Qtype: dns.TypeTXT,
			Answer: []dns.RR{test.TXT(`onlytxt.aminm.com. 300 IN TXT "just-txt"`)},
		}); err != nil {
			t.Fatal(err)
		}

		respCNAME := query(t, "onlytxt.aminm.com.", dns.TypeCNAME)
		if respCNAME.Rcode != dns.RcodeSuccess || len(respCNAME.Answer) != 0 {
			t.Fatalf("expected NODATA for CNAME on pure TXT, got rcode=%s answer=%v", dns.RcodeToString[respCNAME.Rcode], respCNAME.Answer)
		}
	})

	t.Run("CNAME without text still chases TXT target", func(t *testing.T) {
		resp := query(t, "txtalias.aminm.com.", dns.TypeTXT)
		checkExact(t, resp, test.Case{
			Qname: "txtalias.aminm.com.", Qtype: dns.TypeTXT,
			Answer: []dns.RR{
				test.CNAME("txtalias.aminm.com. 300 IN CNAME onlytxt.aminm.com."),
				test.TXT(`onlytxt.aminm.com. 300 IN TXT "just-txt"`),
			},
		})
	})
}
