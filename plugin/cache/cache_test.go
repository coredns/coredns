package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func cacheMsg(m *dns.Msg, tc test.Case) *dns.Msg {
	m.RecursionAvailable = tc.RecursionAvailable
	m.AuthenticatedData = tc.AuthenticatedData
	m.CheckingDisabled = tc.CheckingDisabled
	m.Authoritative = tc.Authoritative
	m.Rcode = tc.Rcode
	m.Truncated = tc.Truncated
	m.Answer = tc.Answer
	m.Ns = tc.Ns
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

// TestCacheInsertion verifies the insertion of items to the cache.
func TestCacheInsertion(t *testing.T) {
	cacheTestCases := []struct {
		name        string
		out         test.Case // the expected message coming "out" of cache
		in          test.Case // the test message going "in" to cache
		shouldCache bool
	}{
		{
			name: "test ad bit cache",
			out: test.Case{
				Qname: "miek.nl.", Qtype: dns.TypeMX,
				Answer: []dns.RR{
					test.MX("miek.nl.	3600	IN	MX	1 aspmx.l.google.com."),
					test.MX("miek.nl.	3600	IN	MX	10 aspmx2.googlemail.com."),
				},
				RecursionAvailable: true,
				AuthenticatedData:  true,
			},
			in: test.Case{
				Qname: "miek.nl.", Qtype: dns.TypeMX,
				Answer: []dns.RR{
					test.MX("miek.nl.	3601	IN	MX	1 aspmx.l.google.com."),
					test.MX("miek.nl.	3601	IN	MX	10 aspmx2.googlemail.com."),
				},
				RecursionAvailable: true,
				AuthenticatedData:  true,
			},
			shouldCache: true,
		},
		{
			name: "test case sensitivity cache",
			out: test.Case{
				Qname: "miek.nl.", Qtype: dns.TypeMX,
				Answer: []dns.RR{
					test.MX("miek.nl.	3600	IN	MX	1 aspmx.l.google.com."),
					test.MX("miek.nl.	3600	IN	MX	10 aspmx2.googlemail.com."),
				},
				RecursionAvailable: true,
				AuthenticatedData:  true,
			},
			in: test.Case{
				Qname: "mIEK.nL.", Qtype: dns.TypeMX,
				Answer: []dns.RR{
					test.MX("miek.nl.	3601	IN	MX	1 aspmx.l.google.com."),
					test.MX("miek.nl.	3601	IN	MX	10 aspmx2.googlemail.com."),
				},
				RecursionAvailable: true,
				AuthenticatedData:  true,
			},
			shouldCache: true,
		},
		{
			name: "test truncated responses shouldn't cache",
			in: test.Case{
				Qname: "miek.nl.", Qtype: dns.TypeMX,
				Answer:    []dns.RR{test.MX("miek.nl.	1800	IN	MX	1 aspmx.l.google.com.")},
				Truncated: true,
			},
			shouldCache: false,
		},
		{
			name: "test dns.RcodeNameError cache",
			out: test.Case{
				Rcode: dns.RcodeNameError,
				Qname: "example.org.", Qtype: dns.TypeA,
				Ns: []dns.RR{
					test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
				},
				RecursionAvailable: true,
			},
			in: test.Case{
				Rcode: dns.RcodeNameError,
				Qname: "example.org.", Qtype: dns.TypeA,
				Ns: []dns.RR{
					test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
				},
				RecursionAvailable: true,
			},
			shouldCache: true,
		},
		{
			name: "test dns.RcodeNameError without SOA does not cache",
			in: test.Case{
				Rcode:              dns.RcodeNameError,
				Qname:              "1.1.168.192.in-addr.arpa.",
				Qtype:              dns.TypePTR,
				RecursionAvailable: true,
			},
			shouldCache: false,
		},
		{
			name: "test dns.RcodeServerFailure cache",
			out: test.Case{
				Rcode: dns.RcodeServerFailure,
				Qname: "example.org.", Qtype: dns.TypeA,
				Ns:                 []dns.RR{},
				RecursionAvailable: true,
			},
			in: test.Case{
				Rcode: dns.RcodeServerFailure,
				Qname: "example.org.", Qtype: dns.TypeA,
				Ns:                 []dns.RR{},
				RecursionAvailable: true,
			},
			shouldCache: true,
		},
		{
			name: "test dns.RcodeNotImplemented cache",
			out: test.Case{
				Rcode: dns.RcodeNotImplemented,
				Qname: "example.org.", Qtype: dns.TypeA,
				Ns:                 []dns.RR{},
				RecursionAvailable: true,
			},
			in: test.Case{
				Rcode: dns.RcodeNotImplemented,
				Qname: "example.org.", Qtype: dns.TypeA,
				Ns:                 []dns.RR{},
				RecursionAvailable: true,
			},
			shouldCache: true,
		},
		{
			name: "test expired RRSIG doesn't cache",
			in: test.Case{
				Qname: "miek.nl.", Qtype: dns.TypeMX,
				Do: true,
				Answer: []dns.RR{
					test.MX("miek.nl.	3600	IN	MX	1 aspmx.l.google.com."),
					test.MX("miek.nl.	3600	IN	MX	10 aspmx2.googlemail.com."),
					test.RRSIG("miek.nl.	1800	IN	RRSIG	MX 8 2 1800 20160521031301 20160421031301 12051 miek.nl. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
				},
				RecursionAvailable: true,
			},
			shouldCache: false,
		},
		{
			name: "test DO bit with RRSIG not expired cache",
			out: test.Case{
				Qname: "example.org.", Qtype: dns.TypeMX,
				Do: true,
				Answer: []dns.RR{
					test.MX("example.org.	3600	IN	MX	1 aspmx.l.google.com."),
					test.MX("example.org.	3600	IN	MX	10 aspmx2.googlemail.com."),
					test.RRSIG("example.org.	3600	IN	RRSIG	MX 8 2 1800 20170521031301 20170421031301 12051 miek.nl. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
				},
				RecursionAvailable: true,
			},
			in: test.Case{
				Qname: "example.org.", Qtype: dns.TypeMX,
				Do: true,
				Answer: []dns.RR{
					test.MX("example.org.	3600	IN	MX	1 aspmx.l.google.com."),
					test.MX("example.org.	3600	IN	MX	10 aspmx2.googlemail.com."),
					test.RRSIG("example.org.	1800	IN	RRSIG	MX 8 2 1800 20170521031301 20170421031301 12051 miek.nl. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
				},
				RecursionAvailable: true,
			},
			shouldCache: true,
		},
		{
			name: "test CD bit cache",
			out: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "dnssec-failed.org.",
				Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.A("dnssec-failed.org. 3600 IN	A	127.0.0.1"),
				},
				CheckingDisabled: true,
			},
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "dnssec-failed.org.",
				Answer: []dns.RR{
					test.A("dnssec-failed.org. 3600 IN	A	127.0.0.1"),
				},
				Qtype:            dns.TypeA,
				CheckingDisabled: true,
			},
			shouldCache: true,
		},
		{
			name: "test negative zone exception shouldn't cache",
			in: test.Case{
				Rcode: dns.RcodeNameError,
				Qname: "neg-disabled.example.org.", Qtype: dns.TypeA,
				Ns: []dns.RR{
					test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
				},
			},
			shouldCache: false,
		},
		{
			name: "test positive zone exception shouldn't cache",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "pos-disabled.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.A("pos-disabled.example.org. 3600 IN	A	127.0.0.1"),
				},
			},
			shouldCache: false,
		},
		{
			name: "test positive zone exception with negative answer cache",
			in: test.Case{
				Rcode: dns.RcodeNameError,
				Qname: "pos-disabled.example.org.", Qtype: dns.TypeA,
				Ns: []dns.RR{
					test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
				},
			},
			out: test.Case{
				Rcode: dns.RcodeNameError,
				Qname: "pos-disabled.example.org.", Qtype: dns.TypeA,
				Ns: []dns.RR{
					test.SOA("example.org. 3600 IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600"),
				},
			},
			shouldCache: true,
		},
		{
			name: "test negative zone exception with positive answer cache",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "neg-disabled.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.A("neg-disabled.example.org. 3600 IN	A	127.0.0.1"),
				},
			},
			out: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "neg-disabled.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.A("neg-disabled.example.org. 3600 IN	A	127.0.0.1"),
				},
			},
			shouldCache: true,
		},
		{
			name: "test NOERROR dangling CNAME chain without SOA does not cache",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target1.example.net."),
					test.CNAME("target1.example.net. 3600 IN CNAME target2.example.net."),
				},
				RecursionAvailable: true,
			},
			shouldCache: false,
		},
		{
			name: "test NOERROR CNAME chain ending in a different type without SOA does not cache",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
					test.AAAA("target.example.net. 3600 IN AAAA ::1"),
				},
				RecursionAvailable: true,
			},
			shouldCache: false,
		},
		{
			name: "test NOERROR MX query CNAME chain without terminal MX without SOA does not cache",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeMX,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
				},
				RecursionAvailable: true,
			},
			shouldCache: false,
		},
		{
			name: "test NOERROR MX query CNAME chain terminating in MX caches",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeMX,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
					test.MX("target.example.net. 3600 IN MX 10 mail.example.net."),
				},
				RecursionAvailable: true,
			},
			out: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeMX,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
					test.MX("target.example.net. 3600 IN MX 10 mail.example.net."),
				},
				RecursionAvailable: true,
			},
			shouldCache: true,
		},
		{
			name: "test NOERROR DNSSEC dangling CNAME chain without SOA does not cache",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Do:    true,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
					test.RRSIG("alias.example.org.	3600	IN	RRSIG	CNAME 8 2 3600 20170521031301 20170421031301 12051 example.org. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
				},
				RecursionAvailable: true,
			},
			shouldCache: false,
		},
		{
			name: "test NOERROR CNAME chain with unrelated A off the chain without SOA does not cache",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
					test.A("unrelated.example.net. 3600 IN A 192.0.2.1"),
				},
				RecursionAvailable: true,
			},
			shouldCache: false,
		},
		{
			name: "test NOERROR lone CNAME answer to an ANY query caches",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeANY,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
				},
				RecursionAvailable: true,
			},
			out: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeANY,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
				},
				RecursionAvailable: true,
			},
			shouldCache: true,
		},
		{
			name: "test NOERROR CNAME chain terminating in A record caches",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
					test.A("target.example.net. 3600 IN A 127.0.0.1"),
				},
				RecursionAvailable: true,
			},
			out: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
					test.A("target.example.net. 3600 IN A 127.0.0.1"),
				},
				RecursionAvailable: true,
			},
			shouldCache: true,
		},
		{
			name: "test NOERROR CNAME chain terminating in A record out of order caches",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.A("target.example.net. 3600 IN A 127.0.0.1"),
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
				},
				RecursionAvailable: true,
			},
			out: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.A("target.example.net. 3600 IN A 127.0.0.1"),
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
				},
				RecursionAvailable: true,
			},
			shouldCache: true,
		},
		{
			name: "test NOERROR CNAME answer to a CNAME query caches",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeCNAME,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
				},
				RecursionAvailable: true,
			},
			out: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeCNAME,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
				},
				RecursionAvailable: true,
			},
			shouldCache: true,
		},
		{
			name: "test NOERROR CNAME chain with two distinct targets at one owner without SOA does not cache",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target1.example.net."),
					test.CNAME("alias.example.org. 3600 IN CNAME target2.example.net."),
					test.A("target1.example.net. 3600 IN A 192.0.2.1"),
				},
				RecursionAvailable: true,
			},
			shouldCache: false,
		},
		{
			name: "test NOERROR CNAME chain with two distinct targets at one owner reversed order without SOA does not cache",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target2.example.net."),
					test.CNAME("alias.example.org. 3600 IN CNAME target1.example.net."),
					test.A("target1.example.net. 3600 IN A 192.0.2.1"),
				},
				RecursionAvailable: true,
			},
			shouldCache: false,
		},
		{
			name: "test NOERROR CNAME loop with co-located A without SOA does not cache",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
					test.CNAME("target.example.net. 3600 IN CNAME alias.example.org."),
					test.A("alias.example.org. 3600 IN A 192.0.2.1"),
				},
				RecursionAvailable: true,
			},
			shouldCache: false,
		},
		{
			name: "test NOERROR CNAME chain with duplicate identical CNAME terminating in A caches",
			in: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
					test.A("target.example.net. 3600 IN A 127.0.0.1"),
				},
				RecursionAvailable: true,
			},
			out: test.Case{
				Rcode: dns.RcodeSuccess,
				Qname: "alias.example.org.", Qtype: dns.TypeA,
				Answer: []dns.RR{
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
					test.CNAME("alias.example.org. 3600 IN CNAME target.example.net."),
					test.A("target.example.net. 3600 IN A 127.0.0.1"),
				},
				RecursionAvailable: true,
			},
			shouldCache: true,
		},
	}
	now, _ := time.Parse(time.UnixDate, "Fri Apr 21 10:51:21 BST 2017")
	utc := now.UTC()

	for _, tc := range cacheTestCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new cache every time to prevent accidental comparison with a previous item.
			c, crr := newTestCache(maxTTL)

			m := tc.in.Msg()
			m = cacheMsg(m, tc.in)

			state := request.Request{W: &test.ResponseWriter{}, Req: m}

			mt, _ := response.Typify(m, utc)
			valid, k := key(state.Name(), m, mt, state.Do(), state.Req.CheckingDisabled)

			if valid {
				// Insert cache entry
				crr.set(m, k, mt, c.pttl)
			}

			// Attempt to retrieve cache entry
			i := c.getIfNotStale(time.Now().UTC(), state, "dns://:53")
			found := i != nil

			if !tc.shouldCache && found {
				t.Fatalf("Cached message that should not have been cached: %s", state.Name())
			}
			if tc.shouldCache && !found {
				t.Fatalf("Did not cache message that should have been cached: %s", state.Name())
			}

			if found {
				resp := i.toMsg(m, time.Now().UTC(), state.Do(), m.AuthenticatedData)

				// TODO: If we incorporate these individual checks into the
				//       test.Header function, we can eliminate them from here.
				if resp.Authoritative != tc.out.Authoritative {
					t.Errorf("Expected Authoritative Answer bit to be %t, but got %t", tc.out.Authoritative, resp.Authoritative)
				}
				if resp.AuthenticatedData != tc.out.AuthenticatedData {
					t.Errorf("Expected Authenticated Data bit to be %t, but got %t", tc.out.AuthenticatedData, resp.AuthenticatedData)
				}
				if resp.RecursionAvailable != tc.out.RecursionAvailable {
					t.Errorf("Expected Recursion Available bit to be %t, but got %t", tc.out.RecursionAvailable, resp.RecursionAvailable)
				}
				if resp.CheckingDisabled != tc.out.CheckingDisabled {
					t.Errorf("Expected Checking Disabled bit to be %t, but got %t", tc.out.CheckingDisabled, resp.CheckingDisabled)
				}

				if err := test.Header(tc.out, resp); err != nil {
					t.Logf("Cache %v", resp)
					t.Error(err)
				}
				if err := test.Section(tc.out, test.Answer, resp.Answer); err != nil {
					t.Logf("Cache %v -- %v", test.Answer, resp.Answer)
					t.Error(err)
				}
				if err := test.Section(tc.out, test.Ns, resp.Ns); err != nil {
					t.Error(err)
				}
				if err := test.Section(tc.out, test.Extra, resp.Extra); err != nil {
					t.Error(err)
				}
			}
		})
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

func TestCacheHonorsConfiguredPositiveMaxTTLAboveDefault(t *testing.T) {
	c := New()
	c.pttl = 2 * time.Hour
	c.minpttl = 0
	c.Next = ttlBackend(24 * 60 * 60)

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	c.ServeDNS(context.TODO(), rec, req)

	if rec.Msg == nil || len(rec.Msg.Answer) == 0 {
		t.Fatalf("expected answer, got %+v", rec.Msg)
	}

	if got, want := rec.Msg.Answer[0].Header().Ttl, uint32(7200); got != want {
		t.Fatalf("expected TTL %d, got %d", want, got)
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

func TestServeFromStaleCacheResponseTTL(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		primeRcode  int
		advance     time.Duration
		expectedTTL uint32
	}{
		{
			name:        "legacy default",
			config:      "serve_stale 1h immediate",
			primeRcode:  dns.RcodeSuccess,
			advance:     2 * time.Minute,
			expectedTTL: 0,
		},
		{
			name:        "immediate positive",
			config:      "serve_stale 1h immediate 30s",
			primeRcode:  dns.RcodeSuccess,
			advance:     2 * time.Minute,
			expectedTTL: 30,
		},
		{
			name:        "exact expiry",
			config:      "serve_stale 1h immediate 30s",
			primeRcode:  dns.RcodeSuccess,
			advance:     time.Minute,
			expectedTTL: 30,
		},
		{
			name:        "immediate negative",
			config:      "serve_stale 1h immediate 25s",
			primeRcode:  dns.RcodeNameError,
			advance:     2 * time.Minute,
			expectedTTL: 25,
		},
		{
			name:        "verify failure",
			config:      "serve_stale 1h verify 0 45s",
			primeRcode:  dns.RcodeSuccess,
			advance:     2 * time.Minute,
			expectedTTL: 45,
		},
		{
			name:        "stale ttl overrides keepttl",
			config:      "serve_stale 1h immediate 17s\nkeepttl",
			primeRcode:  dns.RcodeSuccess,
			advance:     2 * time.Minute,
			expectedTTL: 17,
		},
		{
			name:        "fresh response is unchanged",
			config:      "serve_stale 1h immediate 30s",
			primeRcode:  dns.RcodeSuccess,
			advance:     10 * time.Second,
			expectedTTL: 50,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			controller := caddy.NewTestController("dns", fmt.Sprintf("cache {\n%s\n}", tc.config))
			c, err := cacheParse(controller)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			c.Zones = []string{"."}

			switch tc.primeRcode {
			case dns.RcodeSuccess:
				c.Next = ttlBackend(60)
			case dns.RcodeNameError:
				c.Next = nxDomainBackend(60)
			default:
				t.Fatalf("unsupported prime rcode %d", tc.primeRcode)
			}

			req := new(dns.Msg)
			req.SetQuestion("cached.org.", dns.TypeA)
			ctx := context.Background()
			stored := time.Now()
			c.now = func() time.Time { return stored }
			if ret, err := c.ServeDNS(ctx, dnstest.NewRecorder(&test.ResponseWriter{}), req); err != nil || ret != tc.primeRcode {
				t.Fatalf("failed to prime cache: rcode=%d, err=%v", ret, err)
			}

			c.now = func() time.Time { return stored.Add(tc.advance) }
			c.Next = servFailBackend(60)
			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			if ret, err := c.ServeDNS(ctx, rec, req.Copy()); err != nil || ret != dns.RcodeSuccess {
				t.Fatalf("unexpected cached response: rcode=%d, err=%v", ret, err)
			}
			if rec.Msg == nil || rec.Msg.Rcode != tc.primeRcode {
				t.Fatalf("expected response rcode %d, got %v", tc.primeRcode, rec.Msg)
			}

			var got uint32
			if tc.primeRcode == dns.RcodeNameError {
				if len(rec.Msg.Ns) == 0 {
					t.Fatalf("expected authority record, got %v", rec.Msg)
				}
				got = rec.Msg.Ns[0].Header().Ttl
			} else {
				if len(rec.Msg.Answer) == 0 {
					t.Fatalf("expected answer record, got %v", rec.Msg)
				}
				got = rec.Msg.Answer[0].Header().Ttl
			}
			if got != tc.expectedTTL {
				t.Fatalf("expected TTL %d, got %d", tc.expectedTTL, got)
			}
		})
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

		switch tt.upstreamRCode {
		case dns.RcodeSuccess:
			c.Next = ttlBackend(tt.upstreamTtl)
		case dns.RcodeServerFailure:
			// Make upstream fail, should now rely on cache during the c.staleUpTo period
			c.Next = servFailBackend(tt.upstreamTtl)
		case dns.RcodeNameError:
			c.Next = nxDomainBackend(tt.upstreamTtl)
		default:
			t.Fatal("upstream code not implemented")
		}

		r := req.Copy()
		r.SetQuestion(tt.name, dns.TypeA)
		ret, _ := c.ServeDNS(ctx, rec, r)
		if ret != tt.expectedRCode {
			t.Errorf("Test %d: expected rcode=%v, got rcode=%v", i, tt.expectedRCode, ret)
			continue
		}
		switch ret {
		case dns.RcodeSuccess:
			recTtl := rec.Msg.Answer[0].Header().Ttl
			if tt.expectedTtl != int(recTtl) {
				t.Errorf("Test %d: expected TTL=%d, got TTL=%d", i, tt.expectedTtl, recTtl)
			}
		case dns.RcodeNameError:
			soaTtl := rec.Msg.Ns[0].Header().Ttl
			if tt.expectedTtl != int(soaTtl) {
				t.Errorf("Test %d: expected TTL=%d, got TTL=%d", i, tt.expectedTtl, soaTtl)
			}
		}
	}
}

func TestServeFromStaleCacheFetchVerifyTimeout(t *testing.T) {
	// Verify that when verifyStaleTimeout is set and the upstream is slow,
	// the client gets the stale entry within ~timeout, while the in-flight
	// verify continues in the background and refreshes the cache.
	c := New()
	c.staleUpTo = 1 * time.Hour
	c.verifyStale = true
	c.verifyStaleTimeout = 50 * time.Millisecond
	c.staleTTL = 30 * time.Second
	c.Next = ttlBackend(120)

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)
	ctx := context.TODO()

	// Prime the cache with a 120s TTL entry.
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	c.ServeDNS(ctx, rec, req)
	if c.pcache.Len() != 1 {
		t.Fatalf("Msg with > 0 TTL should have been cached")
	}

	// Move forward past the TTL so the entry is stale.
	c.now = func() time.Time { return time.Now().Add(3 * time.Minute) }

	// Swap in a slow backend that takes longer than the verify timeout.
	bgDone := make(chan struct{})
	c.Next = slowTTLBackend(60, 200*time.Millisecond, bgDone)

	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	start := time.Now()
	ret, err := c.ServeDNS(ctx, rec, req.Copy())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ret != dns.RcodeSuccess {
		t.Fatalf("expected RcodeSuccess, got %d", ret)
	}
	if elapsed > 150*time.Millisecond {
		t.Errorf("expected response within ~timeout (50ms); took %v", elapsed)
	}
	if rec.Msg == nil || len(rec.Msg.Answer) == 0 {
		t.Fatalf("expected an answer, got %+v", rec.Msg)
	}
	if got := rec.Msg.Answer[0].Header().Ttl; got != 30 {
		t.Errorf("expected stale TTL=30, got %d", got)
	}

	// Wait for the background verify to complete.
	select {
	case <-bgDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("background verify never completed")
	}
}

func TestServeFromStaleCacheFetchVerifyTimeoutFastUpstream(t *testing.T) {
	// When the upstream answers within the verify timeout, the client should
	// receive the freshly verified response (not a stale one).
	c := New()
	c.staleUpTo = 1 * time.Hour
	c.verifyStale = true
	c.verifyStaleTimeout = 500 * time.Millisecond
	c.Next = ttlBackend(120)

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)
	ctx := context.TODO()

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	c.ServeDNS(ctx, rec, req)
	if c.pcache.Len() != 1 {
		t.Fatalf("Msg with > 0 TTL should have been cached")
	}

	c.now = func() time.Time { return time.Now().Add(3 * time.Minute) }
	// Fast upstream returning fresh TTL=200.
	c.Next = ttlBackend(200)

	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	ret, err := c.ServeDNS(ctx, rec, req.Copy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ret != dns.RcodeSuccess {
		t.Fatalf("expected RcodeSuccess, got %d", ret)
	}
	if rec.Msg == nil || len(rec.Msg.Answer) == 0 {
		t.Fatalf("expected an answer, got %+v", rec.Msg)
	}
	if got := rec.Msg.Answer[0].Header().Ttl; got != 200 {
		t.Errorf("expected fresh TTL=200, got %d", got)
	}
	if !rec.Msg.Authoritative {
		t.Error("expected cached fresh response to preserve authoritative cache reply shaping")
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

	// Add some answers since these need to be duplicated when
	// serving a cached response.
	answer := []dns.RR{
		test.MX("miek.nl.	3601	IN	MX	1 aspmx.l.google.com."),
		test.MX("miek.nl.	3601	IN	MX	10 aspmx2.googlemail.com."),
	}
	reqs := make([]*dns.Msg, 5)
	for i, q := range []string{"example1", "example2", "a", "b", "ddd"} {
		reqs[i] = new(dns.Msg)
		reqs[i].SetQuestion(q+".example.org.", dns.TypeA)
		reqs[i].Answer = answer
	}
	b.ResetTimer()

	rw := &test.ResponseWriter{}
	j := 0
	for b.Loop() {
		req := reqs[j]
		c.ServeDNS(ctx, rw, req)
		j = (j + 1) % 5
	}
}

func BackendHandler() plugin.Handler {
	return plugin.HandlerFunc(func(_ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
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

func authoritativeBackend() plugin.Handler {
	return plugin.HandlerFunc(func(_ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Response = true
		m.Authoritative = true

		owner := m.Question[0].Name
		m.Answer = []dns.RR{test.A(owner + " 303 IN A 127.0.0.53")}

		w.WriteMsg(m)
		return dns.RcodeSuccess, nil
	})
}

func nxDomainBackend(ttl int) plugin.Handler {
	return plugin.HandlerFunc(func(_ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Response, m.RecursionAvailable = true, true

		m.Ns = []dns.RR{test.SOA(fmt.Sprintf("example.org. %d IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600", ttl))}

		m.Rcode = dns.RcodeNameError
		w.WriteMsg(m)
		return dns.RcodeNameError, nil
	})
}

func ttlBackend(ttl int) plugin.Handler {
	return plugin.HandlerFunc(func(_ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Response, m.RecursionAvailable = true, true

		m.Answer = []dns.RR{test.A(fmt.Sprintf("%s %d IN A 127.0.0.53", r.Question[0].Name, ttl))}
		w.WriteMsg(m)
		return dns.RcodeSuccess, nil
	})
}

func servFailBackend(ttl int) plugin.Handler {
	return plugin.HandlerFunc(func(_ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Response, m.RecursionAvailable = true, true

		m.Ns = []dns.RR{test.SOA(fmt.Sprintf("example.org. %d IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600", ttl))}

		m.Rcode = dns.RcodeServerFailure
		w.WriteMsg(m)
		return dns.RcodeServerFailure, nil
	})
}

// slowTTLBackend wraps ttlBackend with a fixed delay to simulate a slow upstream.
// done is closed once the response is written so callers can synchronise with the
// background goroutine.
func slowTTLBackend(ttl int, delay time.Duration, done chan<- struct{}) plugin.Handler {
	return plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return dns.RcodeServerFailure, ctx.Err()
		}
		m := new(dns.Msg)
		m.SetReply(r)
		m.Response, m.RecursionAvailable = true, true
		m.Answer = []dns.RR{test.A(fmt.Sprintf("%s %d IN A 127.0.0.53", r.Question[0].Name, ttl))}
		w.WriteMsg(m)
		if done != nil {
			close(done)
		}
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
	_, k := key(qname, w.Msg, response.NoError, state.Do(), state.Req.CheckingDisabled)
	i, _ := c.pcache.Get(k)
	if i.wildcard != wildcard {
		t.Errorf("expected wildcard response to enter cache with cache item's wildcard = %q, got %q", wildcard, i.wildcard)
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
		t.Errorf("after retrieving wildcard item from cache, expected \"zone/wildcard\" metadata value to be %q, got %q", wildcard, i.wildcard)
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

// TestCacheSeparation verifies whether the cache maintains separation for specific DNS query types and options.
func TestCacheSeparation(t *testing.T) {
	now, _ := time.Parse(time.UnixDate, "Fri Apr 21 10:51:21 BST 2017")
	utc := now.UTC()

	testCases := []struct {
		name         string
		initial      test.Case
		query        test.Case
		expectCached bool // if a cache entry should be found before inserting
	}{
		{
			name: "query type should be unique",
			initial: test.Case{
				Qname: "example.org.",
				Qtype: dns.TypeA,
			},
			query: test.Case{
				Qname: "example.org.",
				Qtype: dns.TypeAAAA,
			},
		},
		{
			name: "DO bit should be unique",
			initial: test.Case{
				Qname: "example.org.",
				Qtype: dns.TypeA,
			},
			query: test.Case{
				Qname: "example.org.",
				Qtype: dns.TypeA,
				Do:    true,
			},
		},
		{
			name: "CD bit should be unique",
			initial: test.Case{
				Qname: "example.org.",
				Qtype: dns.TypeA,
			},
			query: test.Case{
				Qname:            "example.org.",
				Qtype:            dns.TypeA,
				CheckingDisabled: true,
			},
		},
		{
			name: "CD bit and DO bit should be unique",
			initial: test.Case{
				Qname: "example.org.",
				Qtype: dns.TypeA,
			},
			query: test.Case{
				Qname:            "example.org.",
				Qtype:            dns.TypeA,
				CheckingDisabled: true,
				Do:               true,
			},
		},
		{
			name: "CD bit, DO bit, and query type should be unique",
			initial: test.Case{
				Qname: "example.org.",
				Qtype: dns.TypeA,
			},
			query: test.Case{
				Qname:            "example.org.",
				Qtype:            dns.TypeMX,
				CheckingDisabled: true,
				Do:               true,
			},
		},
		{
			name: "authoritative answer bit should NOT be unique",
			initial: test.Case{
				Qname: "example.org.",
				Qtype: dns.TypeA,
			},
			query: test.Case{
				Qname:         "example.org.",
				Qtype:         dns.TypeA,
				Authoritative: true,
			},
			expectCached: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			crr := &ResponseWriter{ResponseWriter: nil, Cache: c}

			// Insert initial cache entry
			m := tc.initial.Msg()
			m = cacheMsg(m, tc.initial)
			state := request.Request{W: &test.ResponseWriter{}, Req: m}

			mt, _ := response.Typify(m, utc)
			valid, k := key(state.Name(), m, mt, state.Do(), state.Req.CheckingDisabled)

			if valid {
				// Insert cache entry
				crr.set(m, k, mt, c.pttl)
			}

			// Attempt to retrieve cache entry
			m = tc.query.Msg()
			m = cacheMsg(m, tc.query)
			state = request.Request{W: &test.ResponseWriter{}, Req: m}

			item := c.getIfNotStale(time.Now().UTC(), state, "dns://:53")
			found := item != nil

			if !tc.expectCached && found {
				t.Fatal("Found cache message should that should not exist prior to inserting")
			}
			if tc.expectCached && !found {
				t.Fatal("Did not find cache message that should exist prior to inserting")
			}
		})
	}
}

// wildcardMetadataBackend mocks a backend that responds with a response for qname synthesized by wildcard
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

func TestServfailDoesNotShadowPositiveCache(t *testing.T) {
	c := New()
	c.staleUpTo = time.Hour // enable serve_stale
	c.failttl = 5 * time.Second
	now := time.Now()
	c.now = func() time.Time { return now }

	// Manually insert a positive entry in pcache (stored 30s ago, TTL 120s -> still valid).
	posMsg := new(dns.Msg)
	posMsg.SetQuestion("example.org.", dns.TypeA)
	posMsg.Response = true
	posMsg.Answer = []dns.RR{test.A("example.org. 120 IN A 127.0.0.53")}
	posItem := newItem(posMsg, now.Add(-30*time.Second), 120*time.Second)
	k := hash("example.org.", dns.TypeA, dns.ClassINET, false, false)
	c.pcache.Add(k, posItem)

	// Manually insert a SERVFAIL entry in ncache (stored just now, TTL 5s).
	failMsg := new(dns.Msg)
	failMsg.SetQuestion("example.org.", dns.TypeA)
	failMsg.Response = true
	failMsg.Rcode = dns.RcodeServerFailure
	failMsg.Ns = []dns.RR{test.SOA("example.org. 5 IN SOA sns.dns.icann.org. noc.dns.icann.org. 2016082540 7200 3600 1209600 3600")}
	failItem := newItem(failMsg, now, 5*time.Second)
	c.ncache.Add(k, failItem)

	// Lookup should prefer the positive entry over the SERVFAIL.
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)
	state := request.Request{W: &test.ResponseWriter{}, Req: req}

	got := c.getIfNotStale(now.Add(time.Second), state, "test")
	if got == nil {
		t.Fatal("expected a cached item, got nil")
	}
	if got.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected positive cache entry (rcode 0), got rcode %d", got.Rcode)
	}
}

func TestPreferPositiveCachePolicy(t *testing.T) {
	c := New()
	c.staleUpTo = time.Hour
	now := time.Now()
	c.now = func() time.Time { return now }

	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)
	state := request.Request{W: &test.ResponseWriter{}, Req: req}
	k := hash(state.Name(), state.QType(), state.QClass(), state.Do(), state.Req.CheckingDisabled)

	positive := new(dns.Msg)
	positive.SetReply(req)
	positive.Answer = []dns.RR{test.A("example.org. 60 IN A 192.0.2.1")}
	c.pcache.Add(k, newItem(positive, now.Add(-2*time.Minute), time.Minute))

	negative := new(dns.Msg)
	negative.SetRcode(req, dns.RcodeNameError)
	negative.Ns = []dns.RR{test.SOA("example.org. 300 IN SOA ns.example.org. hostmaster.example.org. 1 7200 3600 1209600 300")}
	c.ncache.Add(k, newItem(negative, now, 5*time.Minute))

	if got := c.getIfNotStale(now, state, "test"); got == nil || got.Rcode != dns.RcodeNameError {
		t.Fatalf("default policy should prefer ncache NXDOMAIN, got %+v", got)
	}

	c.preferPositive = true
	if got := c.getIfNotStale(now, state, "test"); got == nil || got.Rcode != dns.RcodeSuccess {
		t.Fatalf("prefer_positive should prefer eligible pcache answer, got %+v", got)
	}
}

func TestPreferPositiveRejectsNonAnswer(t *testing.T) {
	c := New()
	c.staleUpTo = time.Hour
	c.preferPositive = true
	now := time.Now()

	req := new(dns.Msg)
	req.SetQuestion("alias.example.org.", dns.TypeA)
	state := request.Request{W: &test.ResponseWriter{}, Req: req}
	k := hash(state.Name(), state.QType(), state.QClass(), state.Do(), state.Req.CheckingDisabled)

	incomplete := new(dns.Msg)
	incomplete.SetReply(req)
	incomplete.Answer = []dns.RR{test.CNAME("alias.example.org. 60 IN CNAME missing.example.org.")}
	c.pcache.Add(k, newItem(incomplete, now, time.Minute))

	negative := new(dns.Msg)
	negative.SetRcode(req, dns.RcodeNameError)
	negative.Ns = []dns.RR{test.SOA("example.org. 300 IN SOA ns.example.org. hostmaster.example.org. 1 7200 3600 1209600 300")}
	c.ncache.Add(k, newItem(negative, now, 5*time.Minute))

	if got := c.getIfNotStale(now, state, "test"); got == nil || got.Rcode != dns.RcodeNameError {
		t.Fatalf("non-answer pcache item must not shadow ncache, got %+v", got)
	}
}

func TestAnswersQuestionStrictEligibility(t *testing.T) {
	chAddress, err := dns.NewRR("cached.org. 60 CH A 192.0.2.20")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		qtype  uint16
		answer []dns.RR
		want   bool
	}{
		{
			name:   "direct A",
			qtype:  dns.TypeA,
			answer: []dns.RR{test.A("cached.org. 60 IN A 192.0.2.10")},
			want:   true,
		},
		{
			name:   "ANY matching owner",
			qtype:  dns.TypeANY,
			answer: []dns.RR{test.A("cached.org. 60 IN A 192.0.2.10")},
			want:   true,
		},
		{
			name:   "ANY unrelated owner",
			qtype:  dns.TypeANY,
			answer: []dns.RR{test.A("unrelated.org. 60 IN A 192.0.2.10")},
			want:   false,
		},
		{
			name:  "duplicate equivalent CNAME targets",
			qtype: dns.TypeCNAME,
			answer: []dns.RR{
				test.CNAME("cached.org. 60 IN CNAME target.org."),
				test.CNAME("cached.org. 60 IN CNAME target.org."),
			},
			want: true,
		},
		{
			name:  "multiple CNAME targets",
			qtype: dns.TypeCNAME,
			answer: []dns.RR{
				test.CNAME("cached.org. 60 IN CNAME first.org."),
				test.CNAME("cached.org. 60 IN CNAME second.org."),
			},
			want: false,
		},
		{
			name:   "wrong RR class",
			qtype:  dns.TypeA,
			answer: []dns.RR{chAddress},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := new(dns.Msg)
			req.SetQuestion("cached.org.", tc.qtype)
			res := new(dns.Msg)
			res.SetReply(req)
			res.Answer = tc.answer

			if got := answersQuestion(res); got != tc.want {
				t.Fatalf("answersQuestion() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestPreferPositiveRetainsLKGAcrossNonAnswerSuccessRefreshes(t *testing.T) {
	c := New()
	c.staleUpTo = time.Hour
	c.preferPositive = true
	now := time.Now()
	c.now = func() time.Time { return now }

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)
	state := request.Request{W: &test.ResponseWriter{}, Req: req}
	writer := &ResponseWriter{Cache: c, state: state, prefetch: true}

	positive := new(dns.Msg)
	positive.SetReply(req)
	positive.Answer = []dns.RR{test.A("cached.org. 60 IN A 192.0.2.10")}
	if err := writer.WriteMsg(positive); err != nil {
		t.Fatal(err)
	}

	now = now.Add(2 * time.Minute)
	refreshes := []*dns.Msg{
		func() *dns.Msg {
			m := new(dns.Msg)
			m.SetReply(req)
			return m
		}(),
		func() *dns.Msg {
			m := new(dns.Msg)
			m.SetReply(req)
			m.Ns = []dns.RR{test.NS("example.org. 60 IN NS ns.example.org.")}
			return m
		}(),
		func() *dns.Msg {
			m := new(dns.Msg)
			m.SetReply(req)
			m.Ns = []dns.RR{test.NS("example.org. 60 IN NS ns.example.org.")}
			m.Extra = []dns.RR{test.A("ns.example.org. 60 IN A 192.0.2.53")}
			return m
		}(),
		func() *dns.Msg {
			m := new(dns.Msg)
			m.SetReply(req)
			m.Extra = []dns.RR{test.A("cached.org. 60 IN A 192.0.2.54")}
			return m
		}(),
	}

	for i, refresh := range refreshes {
		if err := writer.WriteMsg(refresh); err != nil {
			t.Fatal(err)
		}
		got := c.getIfNotStale(now, state, "test")
		if got == nil || !got.answersQuestion(state) {
			t.Fatalf("refresh %d lost last-known-good answer: %+v", i, got)
		}
		if address := got.Answer[0].(*dns.A).A.String(); address != "192.0.2.10" {
			t.Fatalf("refresh %d returned %s, want 192.0.2.10", i, address)
		}
	}
}

func TestPreferPositiveDoesNotServeLKGOutsideStaleWindow(t *testing.T) {
	c := New()
	c.staleUpTo = time.Hour
	c.preferPositive = true
	now := time.Now()

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)
	state := request.Request{W: &test.ResponseWriter{}, Req: req}
	k := hash(state.Name(), state.QType(), state.QClass(), state.Do(), state.Req.CheckingDisabled)

	positive := new(dns.Msg)
	positive.SetReply(req)
	positive.Answer = []dns.RR{test.A("cached.org. 60 IN A 192.0.2.10")}
	lastKnownGood := newItem(positive, now.Add(-2*time.Hour), time.Minute)

	empty := new(dns.Msg)
	empty.SetReply(req)
	current := newItem(empty, now, time.Minute)
	current.lastKnownGood = lastKnownGood
	c.pcache.Add(k, current)

	negative := new(dns.Msg)
	negative.SetRcode(req, dns.RcodeNameError)
	negative.Ns = []dns.RR{test.SOA("example.org. 300 IN SOA ns.example.org. hostmaster.example.org. 1 7200 3600 1209600 300")}
	c.ncache.Add(k, newItem(negative, now, 5*time.Minute))

	if got := c.getIfNotStale(now, state, "test"); got == nil || got.Rcode != dns.RcodeNameError {
		t.Fatalf("expected current NXDOMAIN after LKG stale window, got %+v", got)
	}
}

func TestPreferPositiveVerifyKeepsStaleOnNonAnswers(t *testing.T) {
	tests := []struct {
		name    string
		backend plugin.Handler
	}{
		{name: "NXDOMAIN", backend: nxDomainBackend(300)},
		{name: "NODATA", backend: plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
			m := new(dns.Msg)
			m.SetReply(r)
			m.Ns = []dns.RR{test.SOA("example.org. 300 IN SOA ns.example.org. hostmaster.example.org. 1 7200 3600 1209600 300")}
			return dns.RcodeSuccess, w.WriteMsg(m)
		})},
		{name: "SERVFAIL", backend: servFailBackend(300)},
		{name: "NOTIMP", backend: plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
			m := new(dns.Msg)
			m.SetRcode(r, dns.RcodeNotImplemented)
			return dns.RcodeNotImplemented, w.WriteMsg(m)
		})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			c.staleUpTo = time.Hour
			c.verifyStale = true
			c.preferPositive = true
			c.Next = ttlBackend(60)

			req := new(dns.Msg)
			req.SetQuestion("cached.org.", dns.TypeA)
			ctx := context.Background()
			c.ServeDNS(ctx, &test.ResponseWriter{}, req)

			c.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
			c.Next = tc.backend

			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			ret, err := c.ServeDNS(ctx, rec, req.Copy())
			if err != nil {
				t.Fatal(err)
			}
			if ret != dns.RcodeSuccess || rec.Msg == nil || rec.Msg.Rcode != dns.RcodeSuccess {
				t.Fatalf("expected stale positive response, got ret=%d msg=%+v", ret, rec.Msg)
			}
			if got := rec.Msg.Answer[0].Header().Ttl; got != 0 {
				t.Fatalf("expected stale TTL 0, got %d", got)
			}
			if c.ncache.Len() != 1 {
				t.Fatalf("expected verified %s to be retained in ncache, got %d entries", tc.name, c.ncache.Len())
			}
		})
	}
}

func TestPreferPositiveVerifyRejectsInvalidFreshAnswers(t *testing.T) {
	modes := []struct {
		name    string
		timeout time.Duration
	}{
		{name: "blocking"},
		{name: "bounded", timeout: time.Second},
	}
	invalidResponses := []struct {
		name  string
		build func(*dns.Msg) *dns.Msg
	}{
		{
			name: "expired RRSIG",
			build: func(req *dns.Msg) *dns.Msg {
				m := new(dns.Msg)
				m.SetReply(req)
				m.SetEdns0(4096, true)
				m.Answer = []dns.RR{
					test.A("cached.org. 60 IN A 192.0.2.20"),
					test.RRSIG("cached.org. 60 IN RRSIG A 8 2 60 20160521031301 20160421031301 12051 cached.org. lAaEzB5teQLLKyDenatmyhca7blLRg9DoGNrhe3NReBZN5C5/pMQk8Jc u25hv2fW23/SLm5IC2zaDpp2Fzgm6Jf7e90/yLcwQPuE7JjS55WMF+HE LEh7Z6AEb+Iq4BWmNhUz6gPxD4d9eRMs7EAzk13o1NYi5/JhfL6IlaYy qkc="),
				}
				return m
			},
		},
		{
			name: "truncated",
			build: func(req *dns.Msg) *dns.Msg {
				m := new(dns.Msg)
				m.SetReply(req)
				m.Truncated = true
				m.Answer = []dns.RR{test.A("cached.org. 60 IN A 192.0.2.20")}
				return m
			},
		},
	}

	for _, mode := range modes {
		for _, invalid := range invalidResponses {
			t.Run(mode.name+"/"+invalid.name, func(t *testing.T) {
				c := New()
				c.staleUpTo = time.Hour
				c.verifyStale = true
				c.verifyStaleTimeout = mode.timeout
				c.preferPositive = true

				now := time.Now().UTC()
				c.now = func() time.Time { return now }
				c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
					m := new(dns.Msg)
					m.SetReply(r)
					m.Answer = []dns.RR{test.A("cached.org. 60 IN A 192.0.2.10")}
					return dns.RcodeSuccess, w.WriteMsg(m)
				})

				req := new(dns.Msg)
				req.SetQuestion("cached.org.", dns.TypeA)
				req.SetEdns0(4096, true)
				if _, err := c.ServeDNS(context.Background(), &test.ResponseWriter{}, req.Copy()); err != nil {
					t.Fatal(err)
				}

				now = now.Add(2 * time.Minute)
				c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
					return dns.RcodeSuccess, w.WriteMsg(invalid.build(r))
				})

				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				ret, err := c.ServeDNS(context.Background(), rec, req.Copy())
				if err != nil {
					t.Fatal(err)
				}
				if ret != dns.RcodeSuccess || rec.Msg == nil || rec.Msg.Rcode != dns.RcodeSuccess {
					t.Fatalf("expected stale positive response, got ret=%d msg=%+v", ret, rec.Msg)
				}
				if len(rec.Msg.Answer) == 0 {
					t.Fatal("expected retained stale answer")
				}
				a, ok := rec.Msg.Answer[0].(*dns.A)
				if !ok || a.A.String() != "192.0.2.10" {
					t.Fatalf("expected retained 192.0.2.10, got %v", rec.Msg.Answer)
				}
				if got := a.Hdr.Ttl; got != 0 {
					t.Fatalf("expected stale TTL 0, got %d", got)
				}
			})
		}
	}
}

func TestServeFromStaleCacheFetchVerifyTimeoutUncacheableResponse(t *testing.T) {
	c := New()
	c.staleUpTo = time.Hour
	c.verifyStale = true
	c.verifyStaleTimeout = time.Second
	c.Next = ttlBackend(60)

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)
	c.ServeDNS(context.Background(), &test.ResponseWriter{}, req)
	c.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	c.Next = plugin.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeNameError)
		return dns.RcodeNameError, w.WriteMsg(m)
	})

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ret, err := c.ServeDNS(context.Background(), rec, req.Copy())
	if err != nil {
		t.Fatal(err)
	}
	if ret != dns.RcodeSuccess || rec.Msg == nil || rec.Msg.Rcode != dns.RcodeNameError {
		t.Fatalf("expected direct uncacheable NXDOMAIN, got ret=%d msg=%+v", ret, rec.Msg)
	}
	for _, section := range [][]dns.RR{rec.Msg.Answer, rec.Msg.Ns, rec.Msg.Extra} {
		for _, rr := range section {
			if rr.Header().Ttl > uint32(maxTTL.Seconds()) {
				t.Fatalf("unexpected wrapped TTL %d", rr.Header().Ttl)
			}
		}
	}
}

func TestCNAMEWithSOAStoredAsNODATA(t *testing.T) {
	c := New()
	req := new(dns.Msg)
	req.SetQuestion("alias.example.org.", dns.TypeA)
	crr := &ResponseWriter{
		Cache:    c,
		state:    request.Request{Req: req},
		prefetch: true,
	}

	res := new(dns.Msg)
	res.SetReply(req)
	res.Answer = []dns.RR{test.CNAME("alias.example.org. 300 IN CNAME missing.example.net.")}
	res.Ns = []dns.RR{test.SOA("example.org. 300 IN SOA ns.example.org. hostmaster.example.org. 1 7200 3600 1209600 300")}

	if err := crr.WriteMsg(res); err != nil {
		t.Fatal(err)
	}
	if c.ncache.Len() != 1 {
		t.Fatalf("expected NODATA in ncache, got %d entries", c.ncache.Len())
	}
	if c.pcache.Len() != 0 {
		t.Fatalf("expected no positive cache entry, got %d", c.pcache.Len())
	}
}

func TestServeFromStaleCacheFetchVerifyTimeoutMetadataIsolation(t *testing.T) {
	c := New()
	c.staleUpTo = time.Hour
	c.verifyStale = true
	c.verifyStaleTimeout = 20 * time.Millisecond
	c.Next = ttlBackend(120)

	req := new(dns.Msg)
	req.SetQuestion("cached.org.", dns.TypeA)

	// Prime the cache with a response that will later become stale.
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if ret, err := c.ServeDNS(context.TODO(), rec, req); err != nil || ret != dns.RcodeSuccess {
		t.Fatalf("failed to prime cache: rcode=%d err=%v", ret, err)
	}
	if c.pcache.Len() != 1 {
		t.Fatalf("Msg with > 0 TTL should have been cached")
	}
	c.now = func() time.Time { return time.Now().Add(3 * time.Minute) }

	// Hold the background verifier until ServeDNS has timed out and returned the
	// stale response. It then writes metadata through the context it received.
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	metadataSet := make(chan bool, 1)
	metadataReceived := make(chan string, 1)

	c.Next = plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		value := ""
		if f := metadata.ValueFunc(ctx, "test/request"); f != nil {
			value = f()
		}
		metadataReceived <- value

		close(started)
		<-release

		metadataSet <- metadata.SetValueFunc(ctx, "test/background", func() string { return "set" })
		m := new(dns.Msg)
		m.SetReply(r)
		m.Response, m.RecursionAvailable = true, true
		m.Answer = []dns.RR{test.A("cached.org. 60 IN A 127.0.0.54")}

		err := w.WriteMsg(m)
		close(done)
		return dns.RcodeSuccess, err
	})

	ctx := metadata.ContextWithMetadata(context.TODO())
	if !metadata.SetValueFunc(ctx, "test/request", func() string {
		return "preserved"
	}) {
		t.Fatal("failed to set request metadata")
	}

	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	ret, err := c.ServeDNS(ctx, rec, req.Copy())

	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("background verifier did not start")
	}

	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background verifier did not finish")
	}

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ret != dns.RcodeSuccess {
		t.Fatalf("expected RcodeSuccess, got %d", ret)
	}

	if got := <-metadataReceived; got != "preserved" {
		t.Fatalf(
			"background verifier did not preserve request metadata: got %q",
			got,
		)
	}

	if !<-metadataSet {
		t.Fatal("background verifier did not receive a metadata-enabled context")
	}

	if f := metadata.ValueFunc(ctx, "test/background"); f != nil {
		t.Fatalf("background verifier mutated foreground metadata: %q", f())
	}
}
