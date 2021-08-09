package forward

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

type testZoneEntry struct {
	question  string
	zoneEntry string
}
type testConfig struct {
	zone    string
	entries []testZoneEntry
}

type testPlugin struct {
	handler dns.HandlerFunc
}

func (tp testPlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	tp.handler(w, r)
	return 0, nil
}

func (tp testPlugin) Name() string { return "testplugin" }

func newTestServer(testConfig testConfig) (*dnsserver.Server, error) {
	c := &dnsserver.Config{
		Zone:      testConfig.zone,
		Transport: "dns",
		Debug:     false,
	}

	p := testPlugin{
		handler: func(w dns.ResponseWriter, r *dns.Msg) {
			ret := new(dns.Msg)
			ret.SetReply(r)
			for _, c := range testConfig.entries {
				if strings.HasPrefix(r.Question[0].Name, c.question) {
					rr, _ := dns.NewRR(c.zoneEntry)
					ret.Answer = append(ret.Answer, rr)
					break
				}
			}
			w.WriteMsg(ret)
		},
	}
	c.AddPlugin(func(next plugin.Handler) plugin.Handler { return p })

	s, err := dnsserver.NewServer("dns://:0", []*dnsserver.Config{c})
	if err != nil {
		return nil, err
	}
	l, err := s.Listen()
	if err != nil {
		return nil, err
	}

	go s.Serve(l)

	s.Addr = l.Addr().String()
	return s, nil
}

func TestUpstreamSingleCNAME(t *testing.T) {
	s, err := newTestServer(testConfig{
		zone: "example.com.",
		entries: []testZoneEntry{
			{
				question:  "addr.example.com.",
				zoneEntry: "cname.example.com. IN CNAME test.example.com.",
			},
			{
				question:  "test.example.com.",
				zoneEntry: "test.example.com. IN A 127.0.0.1",
			},
		},
	})

	defer s.Stop()
	if err != nil {
		t.Fatalf("Failed to create test DNS server, %+v", err)
	}
	c := caddy.NewTestController("dns",
		fmt.Sprintf(`forward . %s {
			force_tcp
			upstream
		}`, s.Addr))
	f, err := parseForward(c)
	if err != nil {
		t.Errorf("Failed to create forwarder: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	m := new(dns.Msg)
	m.SetQuestion("addr.example.com.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	ctx := context.WithValue(context.Background(), dnsserver.Key{}, s)
	if _, err := f.ServeDNS(ctx, rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}

	if rec.Msg.Answer == nil || len(rec.Msg.Answer) <= 0 {
		t.Error("Expected to receive valid answer, but didn't")
	}

	if rec.Msg.Answer[0].Header().Rrtype != dns.TypeA {
		t.Errorf("Expected dns.TypeA(%d), got %d", dns.TypeA, rec.Msg.Answer[0].Header().Rrtype)
	}

	a := rec.Msg.Answer[0].(*dns.A)
	if a.A.String() != "127.0.0.1" {
		t.Errorf("Expected address 127.0.0.1, got %s", a.A.String())
	}
}

func TestUpstreamMultipleCNAME(t *testing.T) {
	s, err := newTestServer(testConfig{
		zone: "example.com.",
		entries: []testZoneEntry{
			{
				question:  "addr.example.com.",
				zoneEntry: "cname.example.com. IN CNAME test1.example.com.",
			},
			{
				question:  "test1.example.com.",
				zoneEntry: "test1.example.com. IN CNAME test2.example.com.",
			},
			{
				question:  "test2.example.com.",
				zoneEntry: "test2.example.com. IN A 127.0.0.1",
			},
		},
	})

	defer s.Stop()
	if err != nil {
		t.Fatalf("Failed to create test DNS server, %+v", err)
	}
	c := caddy.NewTestController("dns",
		fmt.Sprintf(`forward . %s {
			force_tcp
			upstream
		}`, s.Addr))
	f, err := parseForward(c)
	if err != nil {
		t.Errorf("Failed to create forwarder: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	m := new(dns.Msg)
	m.SetQuestion("addr.example.com.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	ctx := context.WithValue(context.Background(), dnsserver.Key{}, s)
	if _, err := f.ServeDNS(ctx, rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}

	if rec.Msg.Answer == nil || len(rec.Msg.Answer) <= 0 {
		t.Error("Expected to receive valid answer, but didn't")
	}

	if rec.Msg.Answer[0].Header().Rrtype != dns.TypeA {
		t.Errorf("Expected dns.TypeA(%d), got %d", dns.TypeA, rec.Msg.Answer[0].Header().Rrtype)
	}

	a := rec.Msg.Answer[0].(*dns.A)
	if a.A.String() != "127.0.0.1" {
		t.Errorf("Expected address 127.0.0.1, got %s", a.A.String())
	}
}

func TestUpstreamFailMaxDepth(t *testing.T) {
	s, err := newTestServer(testConfig{
		zone: "example.com.",
		entries: []testZoneEntry{
			{
				question:  "addr.example.com.",
				zoneEntry: "cname.example.com. IN CNAME test1.example.com.",
			},
			{
				question:  "test1.example.com.",
				zoneEntry: "test1.example.com. IN CNAME test2.example.com.",
			},
			{
				question:  "test2.example.com.",
				zoneEntry: "test2.example.com. IN A 127.0.0.1",
			},
		},
	})

	defer s.Stop()
	if err != nil {
		t.Fatalf("Failed to create test DNS server, %+v", err)
	}
	c := caddy.NewTestController("dns",
		fmt.Sprintf(`forward . %s {
			force_tcp
			upstream max_depth 1
		}`, s.Addr))
	f, err := parseForward(c)
	if err != nil {
		t.Errorf("Failed to create forwarder: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	m := new(dns.Msg)
	m.SetQuestion("addr.example.com.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	ctx := context.WithValue(context.Background(), dnsserver.Key{}, s)
	if _, err := f.ServeDNS(ctx, rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}

	if rec.Msg.Answer == nil || len(rec.Msg.Answer) <= 0 {
		t.Error("Expected to receive valid answer, but didn't")
	}

	if rec.Msg.Answer[0].Header().Rrtype != dns.TypeCNAME {
		t.Errorf("Expected dns.TypeCNAME(%d), got %d", dns.TypeCNAME, rec.Msg.Answer[0].Header().Rrtype)
	}

	cname := rec.Msg.Answer[0].(*dns.CNAME)
	if cname.Target != "test1.example.com." {
		t.Errorf("Expected CNAME test1.example.com., got %s", cname.Target)
	}
}

func TestUpstreamFailOnCircularReference(t *testing.T) {
	s, err := newTestServer(testConfig{
		zone: "example.com.",
		entries: []testZoneEntry{
			{
				question:  "addr.example.com.",
				zoneEntry: "cname.example.com. IN CNAME test1.example.com.",
			},
			{
				question:  "test1.example.com.",
				zoneEntry: "test1.example.com. IN CNAME test2.example.com.",
			},
			{
				question:  "test2.example.com.",
				zoneEntry: "test2.example.com. IN CNAME test1.example.com.",
			},
		},
	})

	defer s.Stop()
	if err != nil {
		t.Fatalf("Failed to create test DNS server, %+v", err)
	}
	c := caddy.NewTestController("dns",
		fmt.Sprintf(`forward . %s {
			force_tcp
			upstream
		}`, s.Addr))
	f, err := parseForward(c)
	if err != nil {
		t.Errorf("Failed to create forwarder: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	m := new(dns.Msg)
	m.SetQuestion("addr.example.com.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	ctx := context.WithValue(context.Background(), dnsserver.Key{}, s)
	if _, err := f.ServeDNS(ctx, rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}

	if rec.Msg.Answer == nil || len(rec.Msg.Answer) <= 0 {
		t.Error("Expected to receive valid answer, but didn't")
	}

	if rec.Msg.Answer[0].Header().Rrtype != dns.TypeCNAME {
		t.Errorf("Expected dns.TypeCNAME(%d), got %d", dns.TypeCNAME, rec.Msg.Answer[0].Header().Rrtype)
	}

	cname := rec.Msg.Answer[0].(*dns.CNAME)
	if cname.Target != "test1.example.com." {
		t.Errorf("Expected CNAME test1.example.com., got %s", cname.Target)
	}
}
