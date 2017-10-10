package forward

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/mholt/caddy"
	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

func TestForward(t *testing.T) {
	config := "forward . %s {\nhealth_check 5ms\n}"
	var counter int64

	backend := dnstest.NewServer(dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		atomic.AddInt64(&counter, 1)
		w.WriteMsg(r) // echo back
	}))
	defer backend.Close()

	c := caddy.NewTestController("test", fmt.Sprintf(config, backend.Addr))

	f, err := parseForward(c)
	if err != nil {
		t.Errorf("Expected no error, got: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	m := new(dns.Msg)
	m.SetQuestion("example.org", dns.TypeAAAA)
	w := &test.ResponseWriter{}

	f.ServeDNS(context.TODO(), w, m)

	time.Sleep(50 * time.Millisecond)
	cnt := atomic.LoadInt64(&counter)
	// With all HCs going on, this should be more than 1
	if cnt < 2 {
		t.Errorf("Expecting more than %d requests, got %d", 1, cnt)
	}
}

func TestForwardHealthCheck(t *testing.T) {
	config := "forward . %s {\nhealth_check 5ms\n}"

	var counter int64
	backend := dnstest.NewServer(dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		// Healthchecks are TCP, don't reply
		state := request.Request{W: w, Req: r}
		if state.Proto() == "tcp" {
			return
		}

		atomic.AddInt64(&counter, 1)
		w.WriteMsg(r) // echo back
	}))
	defer backend.Close()

	c := caddy.NewTestController("test", fmt.Sprintf(config, backend.Addr))

	f, err := parseForward(c)
	if err != nil {
		t.Errorf("Expected no error, got: %s", err)
	}
	f.OnStartup()
	defer f.OnShutdown()

	m := new(dns.Msg)
	m.SetQuestion("example.org", dns.TypeAAAA)
	w := &test.ResponseWriter{}

	// wait for HCs
	time.Sleep(50 * time.Millisecond)

	f.ServeDNS(context.TODO(), w, m)

	// Wait for reply to be processed
	time.Sleep(50 * time.Millisecond)

	cnt := atomic.LoadInt64(&counter)
	if cnt != 0 {
		t.Errorf("Expecting no requests for bad upstream, got %d", cnt)
	}
}

func TestIsAllowedDomain(t *testing.T) {
	f := Forward{from: ".", ignored: []string{"example.net."}}

	if x := f.isAllowedDomain("."); x != true {
		t.Errorf("Expected true, got %t for .", x)
	}
	if x := f.isAllowedDomain("www.example.org."); x != true {
		t.Errorf("Expected true, got %t for www.example.org.", x)
	}
	if x := f.isAllowedDomain("example.net."); x != false {
		t.Errorf("Expected false, got %t for example.net.", x)
	}
	if x := f.isAllowedDomain("www.example.net."); x != false {
		t.Errorf("Expected false, got %t for www.example.net.", x)
	}
}
