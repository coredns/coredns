package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type cacheTestCase struct {
	test.Case
	in                 test.Case
	AuthenticatedData  bool
	RecursionAvailable bool
	Truncated          bool
	shouldCache        bool
}

var cacheTestCases = []cacheTestCase{
	{
		RecursionAvailable: true, AuthenticatedData: true,
		Case: test.Case{
			Qname: "miek.nl.", Qtype: dns.TypeMX,
			Answer: []dns.RR{
				test.MX("miek.nl.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("miek.nl.	3600	IN	MX	10 aspmx2.googlemail.com."),
			},
		},
		in: test.Case{
			Qname: "miek.nl.", Qtype: dns.TypeMX,
			Answer: []dns.RR{
				test.MX("miek.nl.	3601	IN	MX	1 aspmx.l.google.com."),
				test.MX("miek.nl.	3601	IN	MX	10 aspmx2.googlemail.com."),
			},
		},
		shouldCache: true,
	},
	{
		RecursionAvailable: true, AuthenticatedData: true,
		Case: test.Case{
			Qname: "mIEK.nL.", Qtype: dns.TypeMX,
			Answer: []dns.RR{
				test.MX("mIEK.nL.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("mIEK.nL.	3600	IN	MX	10 aspmx2.googlemail.com."),
			},
		},
		in: test.Case{
			Qname: "mIEK.nL.", Qtype: dns.TypeMX,
			Answer: []dns.RR{
				test.MX("mIEK.nL.	3601	IN	MX	1 aspmx.l.google.com."),
				test.MX("mIEK.nL.	3601	IN	MX	10 aspmx2.googlemail.com."),
			},
		},
		shouldCache: true,
	},
	{
		Truncated: true,
		Case: test.Case{
			Qname: "miek.nl.", Qtype: dns.TypeMX,
			Answer: []dns.RR{test.MX("miek.nl.	1800	IN	MX	1 aspmx.l.google.com.")},
		},
		in:          test.Case{},
		shouldCache: false,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Rcode: dns.RcodeNameError,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{
				test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
			},
		},
		in: test.Case{
			Rcode: dns.RcodeNameError,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{
				test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
			},
		},
		shouldCache: true,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Rcode: dns.RcodeServerFailure,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{},
		},
		in: test.Case{
			Rcode: dns.RcodeServerFailure,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{},
		},
		shouldCache: true,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Rcode: dns.RcodeNotImplemented,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{},
		},
		in: test.Case{
			Rcode: dns.RcodeNotImplemented,
			Qname: "example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{},
		},
		shouldCache: true,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Qname: "miek.nl.", Qtype: dns.TypeMX,
			Do: true,
			Answer: []dns.RR{
				test.MX("miek.nl.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("miek.nl.	3600	IN	MX	10 aspmx2.googlemail.com."),
				test.RRSIG("miek.nl.	3600	IN	RRSIG	MX 8 2 1800 20160521031301 20160421031301 12051 miek.nl. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
			},
		},
		in: test.Case{
			Qname: "miek.nl.", Qtype: dns.TypeMX,
			Do: true,
			Answer: []dns.RR{
				test.MX("miek.nl.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("miek.nl.	3600	IN	MX	10 aspmx2.googlemail.com."),
				test.RRSIG("miek.nl.	1800	IN	RRSIG	MX 8 2 1800 20160521031301 20160421031301 12051 miek.nl. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
			},
		},
		shouldCache: false,
	},
	{
		RecursionAvailable: true,
		Case: test.Case{
			Qname: "example.org.", Qtype: dns.TypeMX,
			Do: true,
			Answer: []dns.RR{
				test.MX("example.org.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("example.org.	3600	IN	MX	10 aspmx2.googlemail.com."),
				test.RRSIG("example.org.	3600	IN	RRSIG	MX 8 2 1800 20170521031301 20170421031301 12051 miek.nl. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
			},
		},
		in: test.Case{
			Qname: "example.org.", Qtype: dns.TypeMX,
			Do: true,
			Answer: []dns.RR{
				test.MX("example.org.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("example.org.	3600	IN	MX	10 aspmx2.googlemail.com."),
				test.RRSIG("example.org.	1800	IN	RRSIG	MX 8 2 1800 20170521031301 20170421031301 12051 miek.nl. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
			},
		},
		shouldCache: true,
	},
}

func cacheMsg(m *dns.Msg, tc cacheTestCase) *dns.Msg {
	m.RecursionAvailable = tc.RecursionAvailable
	m.AuthenticatedData = tc.AuthenticatedData
	m.Authoritative = true
	m.Rcode = tc.Rcode
	m.Truncated = tc.Truncated
	m.Answer = tc.in.Answer
	m.Ns = tc.in.Ns
	// m.Extra = tc.in.Extra don't copy Extra, because we don't care and fake EDNS0 DO with tc.Do.
	return m
}

func newTestCache(ttl time.Duration) (*Cache, *ResponseWriter) {
	c := New()
	c.pttl = ttl
	c.nttl = ttl

	crr := &ResponseWriter{ResponseWriter: nil, Cache: c}
	return c, crr
}

func TestCache(t *testing.T) {
	now, _ := time.Parse(time.UnixDate, "Fri Apr 21 10:51:21 BST 2017")
	utc := now.UTC()

	c, crr := newTestCache(maxTTL)

	for _, tc := range cacheTestCases {
		m := tc.in.Msg()
		m = cacheMsg(m, tc)

		state := request.Request{W: &test.ResponseWriter{}, Req: m}

		mt, _ := response.Typify(m, utc)
		valid, k := key(state.Name(), m, mt, state.Do())

		if valid {
			crr.set(m, k, mt, c.pttl)
		}

		i, _ := c.get(time.Now().UTC(), state, "dns://:53")
		ok := i != nil

		if ok != tc.shouldCache {
			t.Errorf("Cached message that should not have been cached: %s", state.Name())
			continue
		}

		if ok {
			resp := i.toMsg(m, time.Now().UTC())

			if err := test.Header(tc.Case, resp); err != nil {
				t.Error(err)
				continue
			}

			if err := test.Section(tc.Case, test.Answer, resp.Answer); err != nil {
				t.Error(err)
			}
			if err := test.Section(tc.Case, test.Ns, resp.Ns); err != nil {
				t.Error(err)
			}
			if err := test.Section(tc.Case, test.Extra, resp.Extra); err != nil {
				t.Error(err)
			}
		}
	}
}

func TestCacheZeroTTL(t *testing.T) {
	c := New()
	c.minpttl = 0
	c.minnttl = 0
	c.Next = ttlBackend(0)

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)
	ctx := context.TODO()

	c.ServeDNS(ctx, &test.ResponseWriter{}, req)
	if c.pcache.Len() != 0 {
		t.Errorf("Msg with 0 TTL should not have been cached")
	}
	if c.ncache.Len() != 0 {
		t.Errorf("Msg with 0 TTL should not have been cached")
	}
}

func TestServeFromStaleCache(t *testing.T) {
	c := New()
	c.Next = ttlBackend(60)

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)
	ctx := context.TODO()

	// Cache example.org.
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	c.serveStale = false
	c.staleUpTo = 1 * time.Hour
	c.ServeDNS(ctx, rec, req)
	if c.pcache.Len() != 1 {
		t.Fatalf("Msg with > 0 TTL should have been cached")
	}

	// No more backend resolutions, just from cache if available.
	c.Next = plugin.HandlerFunc(func(context.Context, dns.ResponseWriter, *dns.Msg) (int, error) {
		return 255, nil // Below, a 255 means we tried querying upstream.
	})

	tests := []struct {
		name           string
		serveStale     bool
		futureMinutes  int
		expectedResult int
	}{
		{"cached.org.", true, 30, 0},
		{"cached.org.", true, 70, 255},
		{"cached.org.", false, 30, 255},
		{"cached.org.", false, 70, 255},

		{"notcached.org.", true, 30, 255},
		{"notcached.org.", true, 70, 255},
		{"notcached.org.", false, 30, 255},
		{"notcached.org.", false, 70, 255},
	}

	for i, tt := range tests {
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		c.now = func() time.Time { return time.Now().Add(time.Duration(tt.futureMinutes) * time.Minute) }
		c.serveStale = tt.serveStale
		r := req.Copy()
		r.SetQuestion(tt.name, dns.TypeA)
		if ret, _ := c.ServeDNS(ctx, rec, r); ret != tt.expectedResult {
			t.Errorf("Test %d: expecting %v; got %v", i, tt.expectedResult, ret)
		}
	}
}

func BenchmarkCacheResponse(b *testing.B) {
	c := New()
	c.prefetch = 1
	c.Next = BackendHandler()

	ctx := context.TODO()

	reqs := make([]*dns.Msg, 5)
	for i, q := range []string{"example1", "example2", "a", "b", "ddd"} {
		reqs[i] = new(dns.Msg)
		reqs[i].SetQuestion(q+".example.org.", dns.TypeA)
	}

	b.StartTimer()

	j := 0
	for i := 0; i < b.N; i++ {
		req := reqs[j]
		c.ServeDNS(ctx, &test.ResponseWriter{}, req)
		j = (j + 1) % 5
	}
}

func BackendHandler() plugin.Handler {
	return plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Response = true
		m.RecursionAvailable = true

		owner := m.Question[0].Name
		m.Answer = []dns.RR{test.A(owner + " 303 IN A 127.0.0.53")}

		w.WriteMsg(m)
		return dns.RcodeSuccess, nil
	})
}

func ttlBackend(ttl int) plugin.Handler {
	return plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Response, m.RecursionAvailable = true, true

		m.Answer = []dns.RR{test.A(fmt.Sprintf("example.org. %d IN A 127.0.0.53", ttl))}
		w.WriteMsg(m)
		return dns.RcodeSuccess, nil
	})
}

func TestComputeTTL(t *testing.T) {
	tests := []struct {
		msgTTL      time.Duration
		minTTL      time.Duration
		maxTTL      time.Duration
		expectedTTL time.Duration
	}{
		{1800 * time.Second, 300 * time.Second, 3600 * time.Second, 1800 * time.Second},
		{299 * time.Second, 300 * time.Second, 3600 * time.Second, 300 * time.Second},
		{299 * time.Second, 0 * time.Second, 3600 * time.Second, 299 * time.Second},
		{3601 * time.Second, 300 * time.Second, 3600 * time.Second, 3600 * time.Second},
	}
	for i, test := range tests {
		ttl := computeTTL(test.msgTTL, test.minTTL, test.maxTTL)
		if ttl != test.expectedTTL {
			t.Errorf("Test %v: Expected ttl %v but found: %v", i, test.expectedTTL, ttl)
		}
	}
}
