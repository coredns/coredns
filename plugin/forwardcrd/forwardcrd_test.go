package forwardcrd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/forward"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestDNSRequestForZone(t *testing.T) {
	k, closeAll := setupForwardCRDTestcase(t, "")
	defer closeAll()

	m := new(dns.Msg)
	m.SetQuestion("crd.test.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := k.ServeDNS(context.Background(), rec, m); err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	if rec.Msg == nil || len(rec.Msg.Answer) != 1 {
		t.Fatal("Expected an answer")
	}

	if x := rec.Msg.Answer[0].Header().Name; x != "crd.test." {
		t.Fatalf("Expected answer header name to be: %s, but got: %s", "crd.test.", x)
	}

	if x := rec.Msg.Answer[0].(*dns.A).A.String(); x != "1.2.3.4" {
		t.Fatalf("Expected answer ip to be: %s, but got: %s", "1.2.3.4", x)
	}

	m = new(dns.Msg)
	m.SetQuestion("other.test.", dns.TypeA)
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := k.ServeDNS(context.Background(), rec, m); err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	if rec.Msg == nil || len(rec.Msg.Answer) != 1 {
		t.Fatal("Expected an answer")
	}

	if x := rec.Msg.Answer[0].Header().Name; x != "other.test." {
		t.Fatalf("Expected answer header name to be: %s, but got: %s", "other.test.", x)
	}

	if x := rec.Msg.Answer[0].(*dns.A).A.String(); x != "1.2.3.4" {
		t.Fatalf("Expected answer ip to be: %s, but got: %s", "1.2.3.4", x)
	}
}

func TestDNSRequestForSubdomain(t *testing.T) {
	k, closeAll := setupForwardCRDTestcase(t, "")
	defer closeAll()

	m := new(dns.Msg)
	m.SetQuestion("foo.crd.test.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := k.ServeDNS(context.Background(), rec, m); err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	if rec.Msg == nil || len(rec.Msg.Answer) != 1 {
		t.Fatal("Expected an answer")
	}

	if x := rec.Msg.Answer[0].Header().Name; x != "foo.crd.test." {
		t.Fatalf("Expected answer header name to be: %s, but got: %s", "foo.crd.test.", x)
	}

	if x := rec.Msg.Answer[0].(*dns.A).A.String(); x != "1.2.3.4" {
		t.Fatalf("Expected answer ip to be: %s, but got: %s", "1.2.3.4", x)
	}
}

func TestDNSRequestForNonexistantZone(t *testing.T) {
	k, closeAll := setupForwardCRDTestcase(t, "")
	defer closeAll()

	m := new(dns.Msg)
	m.SetQuestion("non-existant-zone.test.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if rcode, err := k.ServeDNS(context.Background(), rec, m); err == nil || rcode != dns.RcodeServerFailure {
		t.Fatalf("Expected to return rcode: %d and to error: %s", rcode, err)
	}
}

func TestDNSRequestForLimitedZones(t *testing.T) {
	k, closeAll := setupForwardCRDTestcase(t, "crd.test.")
	defer closeAll()

	m := new(dns.Msg)
	m.SetQuestion("crd.test.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := k.ServeDNS(context.Background(), rec, m); err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	if rec.Msg == nil || len(rec.Msg.Answer) != 1 {
		t.Fatal("Expected an answer")
	}

	m = new(dns.Msg)
	m.SetQuestion("sub.crd.test.", dns.TypeA)
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := k.ServeDNS(context.Background(), rec, m); err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	if rec.Msg == nil || len(rec.Msg.Answer) != 1 {
		t.Fatal("Expected an answer")
	}

	m = new(dns.Msg)
	m.SetQuestion("other.test.", dns.TypeA)
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	if rcode, err := k.ServeDNS(context.Background(), rec, m); err == nil || rcode != dns.RcodeServerFailure {
		t.Fatalf("Expected to return rcode: %d and to error: %s", rcode, err)
	}

	m = new(dns.Msg)
	m.SetQuestion("test.", dns.TypeA)
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	if rcode, err := k.ServeDNS(context.Background(), rec, m); err == nil || rcode != dns.RcodeServerFailure {
		t.Fatalf("Expected to return rcode: %d and to error: %s", rcode, err)
	}
}

func setupForwardCRDTestcase(t *testing.T, zone string) (*ForwardCRD, func()) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A(fmt.Sprintf("%s IN A 1.2.3.4", strings.ToLower(r.Question[0].Name))))
		w.WriteMsg(ret)
	})

	c := caddy.NewTestController("dns", fmt.Sprintf("forwardcrd %s", zone))
	c.ServerBlockKeys = []string{"."}
	k, err := parseForwardCRD(c)
	if err != nil {
		t.Errorf("Expected not to error: %s", err)
	}

	k.APIConn = &TestController{}

	forwardCRDTest, err := forward.NewWithConfig(forward.ForwardConfig{
		From: "crd.test",
		To:   []string{s.Addr},
	})
	if err != nil {
		t.Errorf("Expected not to error: %s", err)
	}

	forwardCRDTest.OnStartup()

	forwardOtherTest, err := forward.NewWithConfig(forward.ForwardConfig{
		From: "other.test.",
		To:   []string{s.Addr},
	})
	if err != nil {
		t.Errorf("Expected not to error: %s", err)
	}

	forwardOtherTest.OnStartup()

	k.pluginInstanceMap.Upsert("default/crd-test", "crd.test", forwardCRDTest)
	k.pluginInstanceMap.Upsert("default/other-test", "other.test", forwardOtherTest)

	closeAll := func() {
		s.Close()
		forwardCRDTest.OnShutdown()
		forwardOtherTest.OnShutdown()
	}
	return k, closeAll
}
