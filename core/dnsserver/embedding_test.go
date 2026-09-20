package dnsserver_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	_ "github.com/coredns/coredns/plugin/bind"
	_ "github.com/coredns/coredns/plugin/forward"
	_ "github.com/coredns/coredns/plugin/whoami"

	"github.com/miekg/dns"
)

// Register once, but outside init, as an embedding host would do before Start.
var registerEmbeddingObserver sync.Once

func TestEmbeddingForward(t *testing.T) {
	configureEmbedding(t)
	upstream := startEmbeddingUpstream(t)
	input := caddy.CaddyfileInput{
		Filepath: "Corefile",
		Contents: fmt.Appendf(nil, `.:0 {
	bind 127.0.0.1
	forward . %s
	test_observe
}
`, upstream),
		ServerTypeName: "dns",
	}

	// Two live instances exercise host-owned plugin state and independent shutdown.
	first, firstObserver, stopFirst := startEmbeddedForwarder(t, input)
	second, secondObserver, _ := startEmbeddedForwarder(t, input)
	for _, instance := range []*caddy.Instance{first, second} {
		instance.StorageMu.RLock()
		observer := instance.Storage[embeddingObserverKey{}].(*embeddingObserver)
		instance.StorageMu.RUnlock()
		for _, network := range []string{"udp", "tcp"} {
			for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
				checkEmbeddedQuery(t, instance, observer, network, qtype)
			}
		}
	}
	stopFirst()
	if firstObserver.stops != 1 || secondObserver.stops != 0 {
		t.Fatalf("shutdown callbacks ran for the wrong instances: first=%d second=%d", firstObserver.stops, secondObserver.stops)
	}
	listener, err := net.Listen("tcp", first.Servers()[0].Addr().String())
	if err != nil {
		t.Fatalf("embedded TCP listener was not released: %v", err)
	}
	listener.Close()
	packet, err := net.ListenPacket("udp", first.Servers()[0].LocalAddr().String())
	if err != nil {
		t.Fatalf("embedded UDP listener was not released: %v", err)
	}
	packet.Close()
	checkEmbeddedQuery(t, second, secondObserver, "udp", dns.TypeA)
	checkEmbeddedQuery(t, second, secondObserver, "tcp", dns.TypeAAAA)
}

func TestEmbeddingInvalidPlugin(t *testing.T) {
	configureEmbedding(t)
	for _, tc := range []struct {
		name       string
		directives []string
		config     string
		wantError  string
	}{
		{
			name:       "imported but not enabled",
			directives: []string{"bind", "test_observe", "forward"},
			config:     ".:0 {\nbind 127.0.0.1\nwhoami\n}\n",
			wantError:  "Unknown directive 'whoami'",
		},
		{
			name:       "enabled but not registered",
			directives: []string{"bind", "test_missing"},
			config:     ".:0 {\nbind 127.0.0.1\ntest_missing\n}\n",
			wantError:  "no action found for directive 'test_missing'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := dnsserver.SetDirectives(tc.directives); err != nil {
				t.Fatal(err)
			}
			instance, err := caddy.Start(caddy.CaddyfileInput{
				Filepath:       "Corefile",
				Contents:       []byte(tc.config),
				ServerTypeName: "dns",
			})
			if err == nil {
				stopEmbeddedInstance(t, instance)
				t.Fatal("expected a configuration error")
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected %q, got %v", tc.wantError, err)
			}
			if len(instance.Servers()) != 0 {
				t.Fatal("invalid configuration opened listeners")
			}
		})
	}
}

func TestEmbeddingDoesNotRegisterCLIFlags(t *testing.T) {
	// Check the original process FlagSet, not an empty replacement which would
	// hide flags registered by imports. The host must not import coremain.
	for _, name := range []string{"conf", "dns.port", "p", "pidfile", "plugins", "version", "quiet"} {
		if flag.Lookup(name) != nil {
			t.Errorf("embedding imports registered the CLI flag %q", name)
		}
	}
}

func configureEmbedding(t *testing.T) {
	t.Helper()
	oldDirectives, oldCaddyQuiet, oldDNSQuiet := dnsserver.Directives, caddy.Quiet, dnsserver.Quiet
	t.Cleanup(func() {
		dnsserver.Directives, caddy.Quiet, dnsserver.Quiet = oldDirectives, oldCaddyQuiet, oldDNSQuiet
	})
	directives := []string{"bind", "test_observe", "forward"}
	if err := dnsserver.SetDirectives(directives); err != nil {
		t.Fatal(err)
	}
	// A host can reuse its input and register a selected plugin after setting
	// the execution order, as long as both happen before startup.
	directives[1] = "test_missing"
	registerEmbeddingObserver.Do(func() {
		plugin.Register("test_observe", setupEmbeddingObserver)
	})
	caddy.Quiet, dnsserver.Quiet = true, true
}

type embeddingObserverKey struct{}

type embeddingObservation struct {
	question dns.Question
	response *dns.Msg
}

type embeddingObserver struct {
	next         plugin.Handler
	config       *dnsserver.Config
	observations chan embeddingObservation
	starts       int
	stops        int
}

func setupEmbeddingObserver(c *caddy.Controller) error {
	for c.Next() {
		if len(c.RemainingArgs()) != 0 {
			return c.ArgErr()
		}
	}
	observer := &embeddingObserver{
		config:       dnsserver.GetConfig(c),
		observations: make(chan embeddingObservation, 1),
	}
	observer.config.AddPlugin(func(next plugin.Handler) plugin.Handler {
		observer.next = next
		return observer
	})
	c.OnStartup(func() error { observer.starts++; return nil })
	c.OnShutdown(func() error { observer.stops++; return nil })
	c.Set(embeddingObserverKey{}, observer)
	return nil
}

func (o *embeddingObserver) Name() string { return "test_observe" }

func (o *embeddingObserver) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	writer := &embeddingResponseWriter{ResponseWriter: w}
	rcode, err := plugin.NextOrFailure(o.Name(), o.next, ctx, writer, r)
	o.observations <- embeddingObservation{question: r.Question[0], response: writer.response}
	return rcode, err
}

type embeddingResponseWriter struct {
	dns.ResponseWriter
	response *dns.Msg
}

func (w *embeddingResponseWriter) WriteMsg(m *dns.Msg) error {
	w.response = m.Copy()
	return w.ResponseWriter.WriteMsg(m)
}

func startEmbeddedForwarder(t *testing.T, input caddy.CaddyfileInput) (*caddy.Instance, *embeddingObserver, func()) {
	t.Helper()
	instance, err := caddy.Start(input)
	if err != nil {
		t.Fatal(err)
	}
	stop := sync.OnceFunc(func() { stopEmbeddedInstance(t, instance) })
	t.Cleanup(stop)
	instance.StorageMu.RLock()
	observer, ok := instance.Storage[embeddingObserverKey{}].(*embeddingObserver)
	instance.StorageMu.RUnlock()
	if !ok {
		t.Fatal("custom plugin was not set up")
	}
	if observer.starts != 1 {
		t.Fatalf("startup callbacks: got %d, want 1", observer.starts)
	}
	handlers := observer.config.Handlers()
	names := make([]string, 0, len(handlers))
	for _, handler := range handlers {
		names = append(names, handler.Name())
	}
	if !slices.Equal(names, []string{"test_observe", "forward"}) {
		t.Fatalf("handler order: got %v, want [test_observe forward]", names)
	}
	return instance, observer, stop
}

func stopEmbeddedInstance(t *testing.T, instance *caddy.Instance) {
	t.Helper()
	shutdownErr := errors.Join(instance.ShutdownCallbacks()...)
	stopErr := instance.Stop()
	instance.Wait()
	if err := errors.Join(shutdownErr, stopErr); err != nil {
		t.Errorf("stop embedded instance: %v", err)
	}
}

func checkEmbeddedQuery(t *testing.T, instance *caddy.Instance, observer *embeddingObserver, network string, qtype uint16) {
	t.Helper()
	server := instance.Servers()[0]
	addr := server.LocalAddr()
	if network == "tcp" {
		addr = server.Addr()
	}
	query := new(dns.Msg)
	query.SetQuestion("embedded.example.", qtype)
	client := &dns.Client{Net: network, Timeout: 2 * time.Second}
	response, _, err := client.Exchange(query, addr.String())
	if err != nil {
		t.Fatal(err)
	}
	wantAnswer := "embedded.example.\t60\tIN\tA\t192.0.2.53"
	if qtype == dns.TypeAAAA {
		wantAnswer = "embedded.example.\t60\tIN\tAAAA\t2001:db8::53"
	}
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 || response.Answer[0].String() != wantAnswer {
		t.Fatalf("unexpected forwarded response: %v", response)
	}
	select {
	case observed := <-observer.observations:
		if observed.question != query.Question[0] || observed.response == nil ||
			observed.response.Rcode != response.Rcode || len(observed.response.Answer) != 1 ||
			observed.response.Answer[0].String() != wantAnswer {
			t.Fatalf("custom plugin did not observe the question and answer: %+v", observed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("custom plugin did not observe the forwarded query")
	}
}

func startEmbeddingUpstream(t *testing.T) string {
	t.Helper()
	var listener net.Listener
	var packet net.PacketConn
	var err error
	// A free TCP port may already be in use by UDP. Reserve both before serving.
	for range 5 {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		packet, err = net.ListenPacket("udp", listener.Addr().String())
		if err == nil {
			break
		}
		listener.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		listener.Close()
		packet.Close()
	})
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		response := new(dns.Msg)
		response.SetReply(r)
		question := r.Question[0]
		header := dns.RR_Header{Name: question.Name, Rrtype: question.Qtype, Class: dns.ClassINET, Ttl: 60}
		switch question.Qtype {
		case dns.TypeA:
			response.Answer = []dns.RR{&dns.A{Hdr: header, A: net.ParseIP("192.0.2.53")}}
		case dns.TypeAAAA:
			response.Answer = []dns.RR{&dns.AAAA{Hdr: header, AAAA: net.ParseIP("2001:db8::53")}}
		}
		if err := w.WriteMsg(response); err != nil {
			t.Errorf("upstream response: %v", err)
		}
	})
	for _, server := range []*dns.Server{
		{Net: "tcp", Listener: listener, Handler: handler},
		{Net: "udp", PacketConn: packet, Handler: handler},
	} {
		ready := make(chan struct{})
		done := make(chan error, 1)
		server.NotifyStartedFunc = func() { close(ready) }
		go func() { done <- server.ActivateAndServe() }()
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
			defer cancel()
			if err := server.ShutdownContext(ctx); err != nil {
				t.Errorf("stop upstream: %v", err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("serve upstream: %v", err)
				}
			case <-ctx.Done():
				t.Error("upstream did not stop")
			}
		})
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			t.Fatal("upstream did not start")
		}
	}
	return listener.Addr().String()
}
