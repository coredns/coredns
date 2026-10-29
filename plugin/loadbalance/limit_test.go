package loadbalance

import (
	"context"
	"crypto"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func limitTestHandler(t *testing.T, config string, next plugin.Handler) LoadBalance {
	t.Helper()
	c := caddy.NewTestController("dns", config)
	if err := setup(c); err != nil {
		t.Fatal(err)
	}
	return dnsserver.GetConfig(c).Plugin[0](next).(LoadBalance)
}

func limitTestRRs(t *testing.T, records ...string) []dns.RR {
	t.Helper()
	out := make([]dns.RR, len(records))
	for i, record := range records {
		var err error
		out[i], err = dns.NewRR(record)
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestLimitAnswersConfig(t *testing.T) {
	for _, policy := range []string{"", "round_robin", "weighted weights"} {
		for _, option := range []string{"1", "2", "65535"} {
			t.Run(policy+"/"+option, func(t *testing.T) {
				limitTestHandler(t, "loadbalance "+policy+" {\nlimit_answers "+option+"\n}", nil)
			})
		}
		for _, option := range []string{"", "0", "-1", "1.5", "many", "999999999999999999999", "1 2", "1\nlimit_answers 2"} {
			t.Run(policy+"/invalid/"+option, func(t *testing.T) {
				_, err := parse(caddy.NewTestController("dns", "loadbalance "+policy+" {\nlimit_answers "+option+"\n}"))
				if err == nil {
					t.Fatal("invalid limit_answers accepted")
				}
			})
		}
	}
}

func TestLimitAnswersRRsets(t *testing.T) {
	answer := limitTestRRs(t,
		"alias.example. 60 IN CNAME target.example.",
		"target.example. 60 IN A 192.0.2.1",
		"target.example. 60 IN AAAA 2001:db8::1",
		"TARGET.example. 60 IN A 192.0.2.2",
		"target.example. 60 IN AAAA 2001:db8::2",
		"other.example. 60 IN A 192.0.2.3",
		"target.example. 60 CH A 192.0.2.4",
		"target.example. 60 IN MX 10 mail1.example.",
		"target.example. 60 IN MX 20 mail2.example.",
		"target.example. 60 IN TXT one",
		"target.example. 60 IN TXT two",
		"old.example. 60 IN DNAME new.example.",
		"_service.example. 60 IN SRV 0 0 80 target.example.",
		"_service.example. 60 IN SRV 0 0 81 target.example.",
		"Ä.example. 60 IN A 192.0.2.5",
		"ä.example. 60 IN A 192.0.2.6",
		`\116arget.example. 60 IN A 192.0.2.7`,
		`\255.example. 60 IN A 192.0.2.8`,
		`\254.example. 60 IN A 192.0.2.9`,
	)
	for _, tc := range []struct {
		name, option string
		want         []int
	}{
		{"one", "limit_answers 1", []int{0, 1, 2, 5, 6, 7, 9, 10, 11, 12, 13, 14, 15, 17, 18}},
		{"two", "limit_answers 2", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 17, 18}},
		{"above size", "limit_answers 100", nil},
		{"disabled", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := new(dns.Msg)
			request.SetQuestion("target.example.", dns.TypeANY)
			response := new(dns.Msg)
			response.SetReply(request)
			response.Answer = answer
			response.Ns = limitTestRRs(t, "example. 60 IN NS ns1.example.", "example. 60 IN NS ns2.example.")
			response.Extra = limitTestRRs(t, "ns1.example. 60 IN A 192.0.2.10", "ns1.example. 60 IN A 192.0.2.11")
			original := response.Copy()
			lb := limitTestHandler(t, "loadbalance {\n"+tc.option+"\n}", plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, _ *dns.Msg) (int, error) {
				return 0, w.WriteMsg(response)
			}))
			// The limiter must retain the ordered prefix of each RRset, even when
			// records from different sets are interleaved.
			lb.shuffle = func(m *dns.Msg) *dns.Msg { return m }
			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			if _, err := lb.ServeDNS(t.Context(), rec, request); err != nil {
				t.Fatal(err)
			}
			want := original.Copy()
			if tc.want != nil {
				want.Answer = nil
				for _, i := range tc.want {
					want.Answer = append(want.Answer, original.Answer[i])
				}
			}
			if !reflect.DeepEqual(rec.Msg.Copy(), want) {
				t.Fatalf("got %s, want %s", rec.Msg, want)
			}
			if !reflect.DeepEqual(answer, original.Answer) {
				t.Fatal("modified the backend-owned answer slice")
			}
		})
	}
}

func TestLimitAnswersBypass(t *testing.T) {
	for _, kind := range []string{"DO", "AD", "RRSIG answer", "RRSIG authority", "RRSIG additional", "SIG", "TSIG", "TC", "AXFR", "IXFR", "SERVFAIL", "NXDOMAIN", "empty question", "NODATA"} {
		t.Run(kind, func(t *testing.T) {
			request := new(dns.Msg)
			request.SetQuestion("example.", dns.TypeA)
			if kind == "DO" {
				request.SetEdns0(1232, true)
			}
			response := new(dns.Msg)
			response.SetReply(request)
			response.Answer = limitTestRRs(t, "example. 60 IN A 192.0.2.1", "example. 60 IN A 192.0.2.2")
			signature := &dns.RRSIG{Hdr: dns.RR_Header{Name: "example.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET}, TypeCovered: dns.TypeA}
			switch kind {
			case "AD":
				response.AuthenticatedData = true
			case "RRSIG answer":
				response.Answer = append(response.Answer, signature)
			case "RRSIG authority":
				response.Ns = []dns.RR{signature}
			case "RRSIG additional":
				response.Extra = []dns.RR{signature}
			case "SIG":
				response.Extra = []dns.RR{&dns.SIG{RRSIG: *signature}}
				response.Extra[0].Header().Rrtype = dns.TypeSIG
			case "TSIG":
				response.Extra = []dns.RR{&dns.TSIG{Hdr: dns.RR_Header{Name: "key.", Rrtype: dns.TypeTSIG, Class: dns.ClassANY}}}
			case "TC":
				response.Truncated = true
			case "AXFR", "IXFR":
				response.Question[0].Qtype = dns.StringToType[kind]
			case "SERVFAIL", "NXDOMAIN":
				response.Rcode = dns.StringToRcode[kind]
			case "empty question":
				response.Question = nil
			case "NODATA":
				response.Answer = nil
			}
			want := response.Copy()
			lb := limitTestHandler(t, "loadbalance {\nlimit_answers 1\n}", plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
				// Downstream plugins may alter EDNS. The original client's DO bit
				// must be remembered before entering the rest of the chain.
				r.Extra = nil
				return 0, w.WriteMsg(response)
			}))
			lb.shuffle = func(m *dns.Msg) *dns.Msg { return m }
			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			if _, err := lb.ServeDNS(t.Context(), rec, request); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rec.Msg.Copy(), want) {
				t.Fatalf("got %s, want %s", rec.Msg, want)
			}
		})
	}
}

func TestLimitAnswersWeighted(t *testing.T) {
	weightFile := filepath.Join(t.TempDir(), "weights")
	if err := os.WriteFile(weightFile, []byte("example.\n192.0.2.1 1\n192.0.2.2 3\n192.0.2.3 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config := "loadbalance weighted " + filepath.ToSlash(weightFile) + " {\nlimit_answers 1\nreload 0s\n}"
	parsed, err := parse(caddy.NewTestController("dns", config))
	if err != nil {
		t.Fatal(err)
	}
	if err := parsed.onStartUpFunc(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { parsed.onShutdownFunc() })
	random := &fakeRandomGen{t: t, expectedLimit: 6}
	parsed.weighted.randomGen = random
	answer := limitTestRRs(t, "example. 60 IN A 192.0.2.1", "example. 60 IN A 192.0.2.2", "example. 60 IN A 192.0.2.3")
	lb := limitTestHandler(t, config, plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = answer
		return 0, w.WriteMsg(m)
	}))
	lb.shuffle = parsed.shuffleFunc
	// Exhaust the RNG's range instead of making a probabilistic assertion.
	counts := make(map[string]int)
	for random.randv = 0; random.randv < 6; random.randv++ {
		q := new(dns.Msg)
		q.SetQuestion("example.", dns.TypeA)
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		if _, err := lb.ServeDNS(t.Context(), rec, q); err != nil {
			t.Fatal(err)
		}
		if len(rec.Msg.Answer) != 1 {
			t.Fatalf("got %d answers, want 1", len(rec.Msg.Answer))
		}
		counts[rec.Msg.Answer[0].(*dns.A).A.String()]++
	}
	if want := map[string]int{"192.0.2.1": 1, "192.0.2.2": 3, "192.0.2.3": 2}; !reflect.DeepEqual(counts, want) {
		t.Fatalf("weighted selections = %v, want %v", counts, want)
	}
}

func TestLimitAnswersSignature(t *testing.T) {
	key := &dns.DNSKEY{Hdr: dns.RR_Header{Name: "example.", Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 60}, Flags: 257, Protocol: 3, Algorithm: dns.ECDSAP256SHA256}
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	answer := limitTestRRs(t, "example. 60 IN A 192.0.2.1", "example. 60 IN A 192.0.2.2")
	signature := &dns.RRSIG{
		Hdr:       dns.RR_Header{Name: "example.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 60},
		Algorithm: key.Algorithm, KeyTag: key.KeyTag(), SignerName: key.Hdr.Name,
		Inception: uint32(time.Now().Add(-time.Hour).Unix()), Expiration: uint32(time.Now().Add(time.Hour).Unix()),
	}
	if err := signature.Sign(private.(crypto.Signer), answer); err != nil {
		t.Fatal(err)
	}
	lb := limitTestHandler(t, "loadbalance {\nlimit_answers 1\n}", plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = append([]dns.RR{signature}, answer...)
		return 0, w.WriteMsg(m)
	}))
	q := new(dns.Msg)
	q.SetQuestion("example.", dns.TypeA) // No DO or AD: the signature itself must protect the RRset.
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := lb.ServeDNS(t.Context(), rec, q); err != nil {
		t.Fatal(err)
	}
	wire, err := rec.Msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	got := new(dns.Msg)
	if err := got.Unpack(wire); err != nil {
		t.Fatal(err)
	}
	var records []dns.RR
	var sig *dns.RRSIG
	for _, rr := range got.Answer {
		if s, ok := rr.(*dns.RRSIG); ok {
			sig = s
		} else {
			records = append(records, rr)
		}
	}
	if sig == nil || len(records) != 2 {
		t.Fatalf("lost signed records: %s", got)
	}
	if err := sig.Verify(key, records); err != nil {
		t.Fatalf("signature no longer verifies: %v", err)
	}
}

func TestLimitAnswersWire(t *testing.T) {
	for _, policy := range []string{"round_robin", "weighted weights"} {
		t.Run(policy, func(t *testing.T) {
			answer := limitTestRRs(t,
				"alias.example. 60 IN CNAME middle.example.",
				"middle.example. 60 IN CNAME target.example.",
				"target.example. 60 IN A 192.0.2.1",
				"target.example. 60 IN A 198.51.100.1",
				"target.example. 60 IN A 192.0.2.2",
			)
			original := new(dns.Msg)
			original.Answer = answer
			before := original.Copy()
			lb := limitTestHandler(t, "loadbalance "+policy+" {\nlimit_answers 1\nprefer 198.51.100.0/24\n}", plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
				m := new(dns.Msg)
				m.SetReply(r)
				m.Answer = answer // Exercise shared backend-owned data.
				return 0, w.WriteMsg(m)
			}))
			server := dnstest.NewMultipleServer(func(w dns.ResponseWriter, r *dns.Msg) {
				if _, err := lb.ServeDNS(t.Context(), w, r); err != nil {
					t.Error(err)
				}
			})
			t.Cleanup(server.Close)
			for _, network := range []string{"udp", "tcp"} {
				t.Run(network, func(t *testing.T) {
					client := &dns.Client{Net: network, Timeout: 5 * time.Second}
					for range 10 {
						q := new(dns.Msg)
						q.SetQuestion("alias.example.", dns.TypeA)
						got, _, err := client.Exchange(q, server.Addr)
						if err != nil {
							t.Fatal(err)
						}
						if got.Rcode != dns.RcodeSuccess || got.Truncated || len(got.Answer) != 3 ||
							got.Answer[0].String() != answer[0].String() || got.Answer[1].String() != answer[1].String() ||
							!strings.Contains(got.Answer[2].String(), "198.51.100.1") {
							t.Fatalf("unexpected limited answer: %s", got)
						}
					}
				})
			}
			if !reflect.DeepEqual(answer, before.Answer) {
				t.Fatal("queries changed backend data")
			}
		})
	}
}
