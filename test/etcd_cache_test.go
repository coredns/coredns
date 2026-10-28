//go:build etcd

package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/etcd/msg"

	"github.com/miekg/dns"
)

// uses some stuff from etcd_tests.go

func TestEtcdCache(t *testing.T) {
	corefile := `.:0 {
		etcd skydns.test {
			path /skydns
		}
		cache skydns.test
	}`

	ex, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer ex.Stop()

	etc := etcdPlugin()

	var ctx = context.TODO()
	for _, serv := range servicesCacheTest {
		set(ctx, t, etc, serv.Key, 0, serv)
		defer delete(ctx, t, etc, serv.Key)
	}

	m := new(dns.Msg)
	m.SetQuestion("b.example.skydns.test.", dns.TypeA)
	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Errorf("Expected to receive reply, but didn't: %s", err)
	}
	checkResponse(t, resp)

	resp, err = dns.Exchange(m, udp)
	if err != nil {
		t.Errorf("Expected to receive reply, but didn't: %s", err)
	}
	checkResponse(t, resp)
	if len(resp.Extra) != 0 {
		t.Errorf("Expected no RRs in additional section, got: %d", len(resp.Extra))
	}
}

func TestEtcdCacheCrossZoneCNAMEConvergence(t *testing.T) {
	corefile := `.:0 {
		cache 30
		etcd example.org example.net {
			path /skydns
			endpoint http://localhost:2379
		}
	}`

	ex, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer ex.Stop()

	etc := etcdPlugin()
	services := []*msg.Service{
		{Host: "b.example.net.", Key: "x1.a.example.org."},
		{Host: "c.example.net.", Key: "x2.a.example.org."},
		{Host: "d.example.org.", Key: "b.example.net."},
		{Host: "d.example.org.", Key: "c.example.net."},
		{Host: "192.0.2.1", Key: "d.example.org."},
	}
	ctx := context.Background()
	for _, serv := range services {
		set(ctx, t, etc, serv.Key, 0, serv)
		defer delete(ctx, t, etc, serv.Key)
	}

	m := new(dns.Msg)
	m.SetQuestion("a.example.org.", dns.TypeA)
	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("parent query failed: %v", err)
	}
	if len(resp.Answer) != 6 {
		t.Fatalf("expected both CNAME branches in the parent answer, got %d records: %v", len(resp.Answer), resp.Answer)
	}

	// Remove the records after the parent lookup. A direct query for c must
	// still get the complete answer cached by the nested lookup.
	delete(ctx, t, etc, services[3].Key)
	delete(ctx, t, etc, services[4].Key)
	for range 2 {
		m.SetQuestion("c.example.net.", dns.TypeA)
		resp, err = dns.Exchange(m, udp)
		if err != nil {
			t.Fatalf("cached sibling query failed: %v", err)
		}
		if len(resp.Answer) != 2 {
			t.Fatalf("expected cached CNAME and address, got %d records: %v", len(resp.Answer), resp.Answer)
		}
		if resp.Answer[0].Header().Rrtype != dns.TypeCNAME || resp.Answer[1].Header().Rrtype != dns.TypeA {
			t.Fatalf("unexpected cached sibling answer: %v", resp.Answer)
		}
	}
}

func TestEtcdCNAMECrossZoneLookupBudget(t *testing.T) {
	corefile := `.:0 {
		cache 30
		etcd example.org example.net {
			path /skydns
			endpoint http://localhost:2379
		}
	}`

	ex, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer ex.Stop()

	etc := etcdPlugin()
	ctx := context.Background()
	for i := 0; i < 33; i++ {
		serv := &msg.Service{
			Host: fmt.Sprintf("target%d.example.net.", i),
			Key:  fmt.Sprintf("x%d.limit.example.org.", i),
		}
		set(ctx, t, etc, serv.Key, 0, serv)
		defer delete(ctx, t, etc, serv.Key)
	}

	m := new(dns.Msg)
	m.SetQuestion("limit.example.org.", dns.TypeA)
	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("budget query failed: %v", err)
	}
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("expected SERVFAIL after exhausting the CNAME lookup budget, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func checkResponse(t *testing.T, resp *dns.Msg) {
	if len(resp.Answer) == 0 {
		t.Fatal("Expected to at least one RR in the answer section, got none")
	}
	if resp.Answer[0].Header().Rrtype != dns.TypeA {
		t.Errorf("Expected RR to A, got: %d", resp.Answer[0].Header().Rrtype)
	}
	if resp.Answer[0].(*dns.A).A.String() != "127.0.0.1" {
		t.Errorf("Expected 127.0.0.1, got: %s", resp.Answer[0].(*dns.A).A.String())
	}
}

var servicesCacheTest = []*msg.Service{
	{Host: "127.0.0.1", Port: 666, Key: "b.example.skydns.test."},
}
