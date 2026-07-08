package forward

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/doh"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestProxy(t *testing.T) {
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
		w.WriteMsg(ret)
	})
	defer s.Close()

	c := caddy.NewTestController("dns", "forward . "+s.Addr)
	fs, err := parseForward(c)
	f := fs[0]
	if err != nil {
		t.Errorf("Failed to create forwarder: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
	if x := rec.Msg.Answer[0].Header().Name; x != "example.org." {
		t.Errorf("Expected %s, got %s", "example.org.", x)
	}
}

func TestProxyTLSFail(t *testing.T) {
	// This is an udp/tcp test server, so we shouldn't reach it with TLS.
	s := dnstest.NewServer(func(w dns.ResponseWriter, r *dns.Msg) {
		ret := new(dns.Msg)
		ret.SetReply(r)
		ret.Answer = append(ret.Answer, test.A("example.org. IN A 127.0.0.1"))
		w.WriteMsg(ret)
	})
	defer s.Close()

	c := caddy.NewTestController("dns", "forward . tls://"+s.Addr)
	fs, err := parseForward(c)
	f := fs[0]
	if err != nil {
		t.Errorf("Failed to create forwarder: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err == nil {
		t.Fatal("Expected *not* to receive reply, but got one")
	}
}

func TestProxyHTTPS(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		msg, err := doh.RequestToMsg(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		ret := new(dns.Msg)
		reply := ret.SetReply(msg)
		reply.Answer = append(reply.Answer, test.A("example.org. IN A 127.0.0.1"))

		buf, err := reply.Pack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", doh.MimeType)
		w.Write(buf)
	}))
	defer s.Close()

	c := caddy.NewTestController("dns", "forward . "+s.URL)
	fs, err := parseForward(c)
	if err != nil {
		t.Errorf("Failed to create forwarder: %s", err)
	}
	f := fs[0]
	f.proxies[0].SetHTTPClient(s.Client())
	f.OnStartup()
	defer f.OnShutdown()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	if _, err := f.ServeDNS(context.TODO(), rec, m); err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
	if x := rec.Msg.Answer[0].Header().Name; x != "example.org." {
		t.Errorf("Expected %s, got %s", "example.org.", x)
	}
}
