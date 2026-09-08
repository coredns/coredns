package ready

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/erratic"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func init() { clog.Discard() }

func TestReady(t *testing.T) {
	rd := &ready{Addr: ":0"}
	e := &erratic.Erratic{}
	plugins.Append(e, "erratic")

	if err := rd.onStartup(); err != nil {
		t.Fatalf("Unable to startup the readiness server: %v", err)
	}

	defer rd.onFinalShutdown()

	address := fmt.Sprintf("http://%s/ready", rd.ln.Addr().String())

	response, err := http.Get(address)
	if err != nil {
		t.Fatalf("Unable to query %s: %v", address, err)
	}
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Invalid status code: expecting %d, got %d", 503, response.StatusCode)
	}
	response.Body.Close()

	// make it ready by giving erratic 3 queries.
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	e.ServeDNS(context.TODO(), &test.ResponseWriter{}, m)
	e.ServeDNS(context.TODO(), &test.ResponseWriter{}, m)
	e.ServeDNS(context.TODO(), &test.ResponseWriter{}, m)

	response, err = http.Get(address)
	if err != nil {
		t.Fatalf("Unable to query %s: %v", address, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("Invalid status code: expecting %d, got %d", 200, response.StatusCode)
	}
	response.Body.Close()

	// make erratic not-ready by giving it more queries, this should not change the process readiness
	e.ServeDNS(context.TODO(), &test.ResponseWriter{}, m)
	e.ServeDNS(context.TODO(), &test.ResponseWriter{}, m)
	e.ServeDNS(context.TODO(), &test.ResponseWriter{}, m)

	response, err = http.Get(address)
	if err != nil {
		t.Fatalf("Unable to query %s: %v", address, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("Invalid status code: expecting %d, got %d", 200, response.StatusCode)
	}
	response.Body.Close()
}

func TestReady_Continuously(t *testing.T) {
	rd := &ready{Addr: ":0"}
	e := &erratic.Erratic{}
	plugins.Append(e, "erratic")
	plugins.keepReadiness = true

	if err := rd.onStartup(); err != nil {
		t.Fatalf("Unable to startup the readiness server: %v", err)
	}

	defer rd.onFinalShutdown()

	address := fmt.Sprintf("http://%s/ready", rd.ln.Addr().String())

	response, err := http.Get(address)
	if err != nil {
		t.Fatalf("Unable to query %s: %v", address, err)
	}
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Invalid status code: expecting %d, got %d", 503, response.StatusCode)
	}
	response.Body.Close()

	// make it ready by giving erratic 3 queries.
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	e.ServeDNS(context.TODO(), &test.ResponseWriter{}, m)
	e.ServeDNS(context.TODO(), &test.ResponseWriter{}, m)
	e.ServeDNS(context.TODO(), &test.ResponseWriter{}, m)

	response, err = http.Get(address)
	if err != nil {
		t.Fatalf("Unable to query %s: %v", address, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("Invalid status code: expecting %d, got %d", 200, response.StatusCode)
	}
	response.Body.Close()

	// make erratic not-ready by giving it more queries, this should change the process readiness
	e.ServeDNS(context.TODO(), &test.ResponseWriter{}, m)
	e.ServeDNS(context.TODO(), &test.ResponseWriter{}, m)
	e.ServeDNS(context.TODO(), &test.ResponseWriter{}, m)

	response, err = http.Get(address)
	if err != nil {
		t.Fatalf("Unable to query %s: %v", address, err)
	}
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Invalid status code: expecting %d, got %d", 503, response.StatusCode)
	}
	response.Body.Close()
}

func TestReadyShutdownDoesNotHoldLockWhileWaitingForHandlers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	accepted := make(chan struct{})
	proceed := make(chan struct{})
	shutdownStarted := make(chan struct{})
	rd := &ready{Addr: ln.Addr().String(), done: true, ln: ln}
	rd.srv = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(accepted)
		<-proceed
		rd.Lock()
		defer rd.Unlock()
		if !rd.done {
			w.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(w, "Shutting down")
			return
		}
		w.WriteHeader(http.StatusOK)
	})}
	rd.srv.RegisterOnShutdown(func() { close(shutdownStarted) })
	go rd.srv.Serve(ln)

	response := make(chan *http.Response, 1)
	requestErr := make(chan error, 1)
	go func() {
		res, err := http.Get("http://" + ln.Addr().String())
		if err != nil {
			requestErr <- err
			return
		}
		response <- res
	}()
	select {
	case <-accepted:
	case err := <-requestErr:
		t.Fatalf("readiness request failed before reaching handler: %v", err)
	case <-time.After(time.Second):
		t.Fatal("readiness request did not reach handler")
	}

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- rd.onFinalShutdown() }()
	select {
	case <-shutdownStarted:
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned before reaching server: %v", err)
	case <-time.After(time.Second):
		t.Fatal("ready server shutdown did not start")
	}
	close(proceed)

	select {
	case err := <-requestErr:
		t.Fatalf("readiness request failed: %v", err)
	case res := <-response:
		defer res.Body.Close()
		if res.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("expected shutdown response %d, got %d", http.StatusServiceUnavailable, res.StatusCode)
		}
	case <-time.After(time.Second):
		t.Fatal("readiness request blocked behind shutdown")
	}

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("shutdown failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ready server shutdown blocked waiting for its handler")
	}
}
