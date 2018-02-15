package cache

import (
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// See https://github.com/kubernetes/dns/issues/121, add some specific tests for those use cases.

func TestMinMsgTTL(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("z.alm.im.", dns.TypeA)
	m.Ns = []dns.RR{
		test.SOA("alm.im.	1800	IN	SOA	ivan.ns.cloudflare.com. dns.cloudflare.com. 2025042470 10000 2400 604800 3600"),
	}

	utc := time.Now().UTC()

	mt, _ := response.Typify(m, utc)
	if mt != response.NoData {
		t.Fatalf("Expected type to be response.NoData, got %s", mt)
	}
	if want, got := 3600*time.Second, minMsgTTL(m, mt); want != got {
		t.Fatalf("Expected minttl duration to be %s, got %s", want, got)
	}

	m.Rcode = dns.RcodeNameError
	mt, _ = response.Typify(m, utc)
	if mt != response.NameError {
		t.Fatalf("Expected type to be response.NameError, got %s", mt)
	}
	if want, got := 3600*time.Second, minMsgTTL(m, mt); want != got {
		t.Fatalf("Expected minttl duration to be %s, got %s", want, got)
	}

	m.Ns = nil
	if want, got := 0*time.Second, minMsgTTL(m, mt); want != got {
		t.Fatalf("Expected minttl duration to be %s, got %s", want, got)
	}
}
