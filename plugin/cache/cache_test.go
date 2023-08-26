package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type cacheTestCase struct {
	test.Case                    // the expected message coming "out" of cache
	in                 test.Case // the test message going "in" to cache
	AuthenticatedData  bool
	RecursionAvailable bool
	Truncated          bool
	shouldCache        bool
}

var cacheTestCases = []cacheTestCase{
	{
		// Test with Authenticated Data bit
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
		// Test case sensitivity
		RecursionAvailable: true, AuthenticatedData: true,
		Case: test.Case{
			Qname: "miek.nl.", Qtype: dns.TypeMX,
			Answer: []dns.RR{
				test.MX("miek.nl.	3600	IN	MX	1 aspmx.l.google.com."),
				test.MX("miek.nl.	3600	IN	MX	10 aspmx2.googlemail.com."),
			},
		},
		in: test.Case{
			Qname: "mIEK.nL.", Qtype: dns.TypeMX,
			Answer: []dns.RR{
				test.MX("miek.nl.	3601	IN	MX	1 aspmx.l.google.com."),
				test.MX("miek.nl.	3601	IN	MX	10 aspmx2.googlemail.com."),
			},
		},
		shouldCache: true,
	},
	{
		// Test truncated responses don't get cached
		Truncated: true,
		Case: test.Case{
			Qname: "miek.nl.", Qtype: dns.TypeMX,
			Answer: []dns.RR{test.MX("miek.nl.	1800	IN	MX	1 aspmx.l.google.com.")},
		},
		in:          test.Case{},
		shouldCache: false,
	},
	{
		// Test dns.RcodeNameError cache
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
		// Test dns.RcodeServerFailure cache
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
		// Test dns.RcodeNotImplemented cache
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
		// Test cache has separate items for query DO Bit value
		// This doesn't cache because this RRSIG is expired and previous tests used DO Bit false
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
		// Test DO Bit with a RRSIG that is not expired
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
	{
		// Test negative zone exception
		in: test.Case{
			Rcode: dns.RcodeNameError,
			Qname: "neg-disabled.example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{
				test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
			},
		},
		Case:        test.Case{},
		shouldCache: false,
	},
	{
		// Test positive zone exception
		in: test.Case{
			Rcode: dns.RcodeSuccess,
			Qname: "pos-disabled.example.org.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("pos-disabled.example.org. 3600 IN	A	127.0.0.1"),
			},
		},
		Case:        test.Case{},
		shouldCache: false,
	},
	{
		// Test positive zone exception with negative answer
		in: test.Case{
			Rcode: dns.RcodeNameError,
			Qname: "pos-disabled.example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{
				test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
			},
		},
		Case: test.Case{
			Rcode: dns.RcodeNameError,
			Qname: "pos-disabled.example.org.", Qtype: dns.TypeA,
			Ns: []dns.RR{
				test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
			},
		},
		shouldCache: true,
	},
	{
		// Test negative zone exception with positive answer
		in: test.Case{
			Rcode: dns.RcodeSuccess,
			Qname: "neg-disabled.example.org.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("neg-disabled.example.org. 3600 IN	A	127.0.0.1"),
			},
		},
		Case: test.Case{
			Rcode: dns.RcodeSuccess,
			Qname: "neg-disabled.example.org.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("neg-disabled.example.org. 3600 IN	A	127.0.0.1"),
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
	crr.nexcept = []string{"neg-disabled.example.org."}
	crr.pexcept = []string{"pos-disabled.example.org."}

	return c, crr
}

func TestCache(t *testing.T) {
	now, _ := time.Parse(time.UnixDate, "Fri Apr 21 10:51:21 BST 2017")
	utc := now.UTC()

	c, crr := newTestCache(maxTTL)

	for n, tc := range cacheTestCases {
		m := tc.in.Msg()
		m = cacheMsg(m, tc)

		state := request.Request{W: &test.ResponseWriter{}, Req: m}

		mt, _ := response.Typify(m, utc)
		valid, k := key(state.Name(), m, mt, state.Do())

		if valid {
			crr.set(m, k, mt, c.pttl)
		}

		i := c.getIgnoreTTL(time.Now().UTC(), state, "dns://:53")
		ok := i != nil

		if !tc.shouldCache && ok {
			t.Errorf("Test %d: Cached message that should not have been cached: %s", n, state.Name())
			continue
		}
		if tc.shouldCache && !ok {
			t.Errorf("Test %d: Did not cache message that should have been cached: %s", n, state.Name())
			continue
		}

		if ok {
			resp := i.toMsg(m, time.Now().UTC(), state.Do(), m.AuthenticatedData)

			if err := test.Header(tc.Case, resp); err != nil {
				t.Logf("Cache %v", resp)
				t.Error(err)
				continue
			}

			if err := test.Section(tc.Case, test.Answer, resp.Answer); err != nil {
				t.Logf("Cache %v -- %v", test.Answer, resp.Answer)
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

func TestCacheServfailTTL0(t *testing.T) {
	c := New()
	c.minpttl = minTTL
	c.minnttl = minNTTL
	c.failttl = 0
	c.Next = servFailBackend(0)

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)
	ctx := context.TODO()

	c.ServeDNS(ctx, &test.ResponseWriter{}, req)
	if c.ncache.Len() != 0 {
		t.Errorf("SERVFAIL response should not have been cached")
	}
}

func TestServeFromStaleCache(t *testing.T) {
	c := New()
	c.Next = ttlBackend(60)

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)
	ctx := context.TODO()

	// Cache cached.org. with 60s TTL
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
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
		futureMinutes  int
		expectedResult int
	}{
		{"cached.org.", 30, 0},
		{"cached.org.", 60, 0},
		{"cached.org.", 70, 255},

		{"notcached.org.", 30, 255},
		{"notcached.org.", 60, 255},
		{"notcached.org.", 70, 255},
	}

	for i, tt := range tests {
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		c.now = func() time.Time { return time.Now().Add(time.Duration(tt.futureMinutes) * time.Minute) }
		r := req.Copy()
		r.SetQuestion(tt.name, dns.TypeA)
		if ret, _ := c.ServeDNS(ctx, rec, r); ret != tt.expectedResult {
			t.Errorf("Test %d: expecting %v; got %v", i, tt.expectedResult, ret)
		}
	}
}

func TestServeFromStaleCacheFetchVerify(t *testing.T) {
	c := New()
	c.Next = ttlBackend(120)

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)
	ctx := context.TODO()

	// Cache cached.org. with 120s TTL
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	c.staleUpTo = 1 * time.Hour
	c.verifyStale = true
	c.ServeDNS(ctx, rec, req)
	if c.pcache.Len() != 1 {
		t.Fatalf("Msg with > 0 TTL should have been cached")
	}

	tests := []struct {
		name          string
		upstreamRCode int
		upstreamTtl   int
		futureMinutes int
		expectedRCode int
		expectedTtl   int
	}{
		// After 1 minutes of initial TTL, we should see a cached response
		{"cached.org.", dns.RcodeSuccess, 200, 1, dns.RcodeSuccess, 60}, // ttl = 120 - 60 -- not refreshed

		// After the 2 more minutes, we should see upstream responses because upstream is available
		{"cached.org.", dns.RcodeSuccess, 200, 3, dns.RcodeSuccess, 200},

		// After the TTL expired, if the server fails we should get the cached entry
		{"cached.org.", dns.RcodeServerFailure, 200, 7, dns.RcodeSuccess, 0},

		// After 1 more minutes, if the server serves nxdomain we should see them (despite being within the serve stale period)
		{"cached.org.", dns.RcodeNameError, 150, 8, dns.RcodeNameError, 150},
	}

	for i, tt := range tests {
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		c.now = func() time.Time { return time.Now().Add(time.Duration(tt.futureMinutes) * time.Minute) }

		if tt.upstreamRCode == dns.RcodeSuccess {
			c.Next = ttlBackend(tt.upstreamTtl)
		} else if tt.upstreamRCode == dns.RcodeServerFailure {
			// Make upstream fail, should now rely on cache during the c.staleUpTo period
			c.Next = servFailBackend(tt.upstreamTtl)
		} else if tt.upstreamRCode == dns.RcodeNameError {
			c.Next = nxDomainBackend(tt.upstreamTtl)
		} else {
			t.Fatal("upstream code not implemented")
		}

		r := req.Copy()
		r.SetQuestion(tt.name, dns.TypeA)
		ret, _ := c.ServeDNS(ctx, rec, r)
		if ret != tt.expectedRCode {
			t.Errorf("Test %d: expected rcode=%v, got rcode=%v", i, tt.expectedRCode, ret)
			continue
		}
		if ret == dns.RcodeSuccess {
			recTtl := rec.Msg.Answer[0].Header().Ttl
			if tt.expectedTtl != int(recTtl) {
				t.Errorf("Test %d: expected TTL=%d, got TTL=%d", i, tt.expectedTtl, recTtl)
			}
		} else if ret == dns.RcodeNameError {
			soaTtl := rec.Msg.Ns[0].Header().Ttl
			if tt.expectedTtl != int(soaTtl) {
				t.Errorf("Test %d: expected TTL=%d, got TTL=%d", i, tt.expectedTtl, soaTtl)
			}
		}
	}
}

func TestNegativeStaleMaskingPositiveCache(t *testing.T) {
	c := New()
	c.staleUpTo = time.Minute * 10
	c.Next = nxDomainBackend(60)

	req := new(dns.Msg)
	qname := "cached.org."
	req.SetQuestion(qname, dns.TypeA)
	ctx := context.TODO()

	// Add an entry to Negative Cache": cached.org. = NXDOMAIN
	expectedResult := dns.RcodeNameError
	if ret, _ := c.ServeDNS(ctx, &test.ResponseWriter{}, req); ret != expectedResult {
		t.Errorf("Test 0 Negative Cache Population: expecting %v; got %v", expectedResult, ret)
	}

	// Confirm item was added to negative cache and not to positive cache
	if c.ncache.Len() == 0 {
		t.Errorf("Test 0 Negative Cache Population: item not added to negative cache")
	}
	if c.pcache.Len() != 0 {
		t.Errorf("Test 0 Negative Cache Population: item added to positive cache")
	}

	// Set the Backend to return non-cachable errors only
	c.Next = plugin.HandlerFunc(func(context.Context, dns.ResponseWriter, *dns.Msg) (int, error) {
		return 255, nil // Below, a 255 means we tried querying upstream.
	})

	// Confirm we get the NXDOMAIN from the negative cache, not the error form the backend
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	req = new(dns.Msg)
	req.SetQuestion(qname, dns.TypeA)
	expectedResult = dns.RcodeNameError
	if c.ServeDNS(ctx, rec, req); rec.Rcode != expectedResult {
		t.Errorf("Test 1 NXDOMAIN from Negative Cache: expecting %v; got %v", expectedResult, rec.Rcode)
	}

	// Jump into the future beyond when the negative cache item would go stale
	// but before the item goes rotten (exceeds serve stale time)
	c.now = func() time.Time { return time.Now().Add(time.Duration(5) * time.Minute) }

	// Set Backend to return a positive NOERROR + A record response
	c.Next = BackendHandler()

	// Make a query for the stale cache item
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	req = new(dns.Msg)
	req.SetQuestion(qname, dns.TypeA)
	expectedResult = dns.RcodeNameError
	if c.ServeDNS(ctx, rec, req); rec.Rcode != expectedResult {
		t.Errorf("Test 2 NOERROR from Backend: expecting %v; got %v", expectedResult, rec.Rcode)
	}

	// Confirm that prefetch removes the negative cache item.
	waitFor := 3
	for i := 1; i <= waitFor; i++ {
		if c.ncache.Len() != 0 {
			if i == waitFor {
				t.Errorf("Test 2 NOERROR from Backend: item still exists in negative cache")
			}
			time.Sleep(time.Second)
			continue
		}
	}

	// Confirm that positive cache has the item
	if c.pcache.Len() != 1 {
		t.Errorf("Test 2 NOERROR from Backend: item missing from positive cache")
	}

	// Backend - Give error only
	c.Next = plugin.HandlerFunc(func(context.Context, dns.ResponseWriter, *dns.Msg) (int, error) {
		return 255, nil // Below, a 255 means we tried querying upstream.
	})

	// Query again, expect that positive cache entry is not masked by a negative cache entry
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	req = new(dns.Msg)
	req.SetQuestion(qname, dns.TypeA)
	expectedResult = dns.RcodeSuccess
	if ret, _ := c.ServeDNS(ctx, rec, req); ret != expectedResult {
		t.Errorf("Test 3 NOERROR from Cache: expecting %v; got %v", expectedResult, ret)
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

func nxDomainBackend(ttl int) plugin.Handler {
	return plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Response, m.RecursionAvailable = true, true

		m.Ns = []dns.RR{test.SOA(fmt.Sprintf("example.org. %d IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600", ttl))}

		m.MsgHdr.Rcode = dns.RcodeNameError
		w.WriteMsg(m)
		return dns.RcodeNameError, nil
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

func servFailBackend(ttl int) plugin.Handler {
	return plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Response, m.RecursionAvailable = true, true

		m.Ns = []dns.RR{test.SOA(fmt.Sprintf("example.org. %d IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600", ttl))}

		m.MsgHdr.Rcode = dns.RcodeServerFailure
		w.WriteMsg(m)
		return dns.RcodeServerFailure, nil
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

func TestCacheWildcardMetadata(t *testing.T) {
	c := New()
	qname := "foo.bar.example.org."
	wildcard := "*.bar.example.org."
	c.Next = wildcardMetadataBackend(qname, wildcard)

	req := new(dns.Msg)
	req.SetQuestion(qname, dns.TypeA)
	state := request.Request{W: &test.ResponseWriter{}, Req: req}

	// 1. Test writing wildcard metadata retrieved from backend to the cache

	ctx := metadata.ContextWithMetadata(context.TODO())
	w := dnstest.NewRecorder(&test.ResponseWriter{})
	c.ServeDNS(ctx, w, req)
	if c.pcache.Len() != 1 {
		t.Errorf("Msg should have been cached")
	}
	_, k := key(qname, w.Msg, response.NoError, state.Do())
	i, _ := c.pcache.Get(k)
	if i.(*item).wildcard != wildcard {
		t.Errorf("expected wildcard response to enter cache with cache item's wildcard = %q, got %q", wildcard, i.(*item).wildcard)
	}

	// 2. Test retrieving the cached item from cache and writing its wildcard value to metadata

	// reset context and response writer
	ctx = metadata.ContextWithMetadata(context.TODO())
	w = dnstest.NewRecorder(&test.ResponseWriter{})

	c.ServeDNS(ctx, w, req)
	f := metadata.ValueFunc(ctx, "zone/wildcard")
	if f == nil {
		t.Fatal("expected metadata func for wildcard response retrieved from cache, got nil")
	}
	if f() != wildcard {
		t.Errorf("after retrieving wildcard item from cache, expected \"zone/wildcard\" metadata value to be %q, got %q", wildcard, i.(*item).wildcard)
	}
}

func TestCacheKeepTTL(t *testing.T) {
	defaultTtl := 60

	c := New()
	c.Next = ttlBackend(defaultTtl)

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)
	ctx := context.TODO()

	// Cache cached.org. with 60s TTL
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	c.keepttl = true
	c.ServeDNS(ctx, rec, req)

	tests := []struct {
		name          string
		futureSeconds int
	}{
		{"cached.org.", 0},
		{"cached.org.", 30},
		{"uncached.org.", 60},
	}

	for i, tt := range tests {
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		c.now = func() time.Time { return time.Now().Add(time.Duration(tt.futureSeconds) * time.Second) }
		r := req.Copy()
		r.SetQuestion(tt.name, dns.TypeA)
		c.ServeDNS(ctx, rec, r)

		recTtl := rec.Msg.Answer[0].Header().Ttl
		if defaultTtl != int(recTtl) {
			t.Errorf("Test %d: expecting TTL=%d, got TTL=%d", i, defaultTtl, recTtl)
		}
	}
}

// wildcardMetadataBackend mocks a backend that reponds with a response for qname synthesized by wildcard
// and sets the zone/wildcard metadata value
func wildcardMetadataBackend(qname, wildcard string) plugin.Handler {
	return plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Response, m.RecursionAvailable = true, true
		m.Answer = []dns.RR{test.A(qname + " 300 IN A 127.0.0.1")}
		metadata.SetValueFunc(ctx, "zone/wildcard", func() string {
			return wildcard
		})
		w.WriteMsg(m)

		return dns.RcodeSuccess, nil
	})
}
