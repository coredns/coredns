package tls

import (
	"context"
	ctls "crypto/tls"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/mholt/acmez/v3/acme"
	"github.com/miekg/dns"
)

func TestParseACMETLS(t *testing.T) {
	root := t.TempDir()
	rootPEM, err := os.ReadFile("test_ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "roots.pem"), rootPEM, 0600); err != nil {
		t.Fatal(err)
	}
	c := caddy.NewTestController("dns", `tls {
		acme DNS.Example. dns.example
		email admin@example.org
		ca https://ca.example/directory
		storage certs
		ca_root roots.pem
		resolver 1.1.1.1:53
	}`)
	cfg := dnsserver.GetConfig(c)
	cfg.Root = root

	if err := setup(c); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if cfg.TLSConfig == nil {
		t.Fatal("TLS config was not set")
	}
	if cfg.TLSConfig.MinVersion != ctls.VersionTLS12 {
		t.Fatalf("minimum TLS version is %d, want TLS 1.2", cfg.TLSConfig.MinVersion)
	}
	if _, err := cfg.TLSConfig.GetCertificate(&ctls.ClientHelloInfo{ServerName: "dns.example"}); !errors.Is(err, errACMENotReady) {
		t.Fatalf("GetCertificate error = %v, want %v", err, errACMENotReady)
	}
	if got := len(cfg.Plugin); got != 1 {
		t.Fatalf("installed %d challenge handlers, want 1", got)
	}

	runtime := c.Get(acmeRuntimeStorageKey{}).(*acmeRuntime)
	if got := len(runtime.entries); got != 1 {
		t.Fatalf("runtime has %d entries, want 1", got)
	}
	for _, entry := range runtime.entries {
		if len(entry.options.domains) != 1 || entry.options.domains[0] != "dns.example" {
			t.Fatalf("domains = %v, want [dns.example]", entry.options.domains)
		}
		if want := filepath.Join(root, "certs"); entry.options.storage != want {
			t.Fatalf("storage = %q, want %q", entry.options.storage, want)
		}
		if want := filepath.Join(root, "roots.pem"); entry.options.caRoot != want {
			t.Fatalf("CA root = %q, want %q", entry.options.caRoot, want)
		}
	}
}

func TestACMEDirectiveOrder(t *testing.T) {
	indexes := make(map[string]int)
	for i, directive := range dnsserver.Directives {
		indexes[directive] = i
	}
	if indexes["tls"] != indexes["dnssec"]+1 {
		t.Fatalf("tls directive index = %d, want immediately after dnssec at %d", indexes["tls"], indexes["dnssec"])
	}
}

func TestParseACMETLSErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"missing domain", "tls", "at least one domain"},
		{"missing acme arguments", "tls {\nacme\n}", "Wrong argument"},
		{"IP identifier", "tls {\nacme 192.0.2.1\n}", "invalid ACME domain"},
		{"invalid CA", "tls {\nacme dns.example\nca ftp://ca.example\n}", "invalid ACME CA URL"},
		{"invalid resolver", "tls {\nacme dns.example\nresolver 1.1.1.1\n}", "invalid ACME resolver"},
		{"invalid resolver port", "tls {\nacme dns.example\nresolver 1.1.1.1:dns\n}", "invalid ACME resolver port"},
		{"invalid wildcard", "tls {\nacme *.*.example\n}", "invalid ACME domain"},
		{"missing CA root", "tls {\nacme dns.example\nca_root missing.pem\n}", "reading ACME CA root"},
		{"duplicate option", "tls {\nacme dns.example\nacme other.example\n}", "only be specified once"},
		{"unknown option", "tls {\nacme dns.example\nprovider example\n}", "unknown ACME option"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tc.input)
			err := setup(c)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("setup error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestACMEDNS01ChallengeHandler(t *testing.T) {
	solver := newACMEDNS01Solver()
	challenge := acme.Challenge{
		Identifier:       acme.Identifier{Type: "dns", Value: "dns.example"},
		KeyAuthorization: "token.thumbprint",
	}
	if err := solver.Present(context.Background(), challenge); err != nil {
		t.Fatal(err)
	}
	if err := solver.Present(context.Background(), challenge); err != nil {
		t.Fatal(err)
	}

	nextCalled := false
	handler := &acmeChallengeHandler{
		solver: solver,
		Next: plugin.HandlerFunc(func(context.Context, dns.ResponseWriter, *dns.Msg) (int, error) {
			nextCalled = true
			return dns.RcodeNameError, nil
		}),
	}
	request := new(dns.Msg)
	request.SetQuestion("_ACME-CHALLENGE.DNS.EXAMPLE.", dns.TypeTXT)
	recorder := dnstest.NewRecorder(&test.ResponseWriter{})
	rcode, err := handler.ServeDNS(context.Background(), recorder, request)
	if err != nil || rcode != dns.RcodeSuccess {
		t.Fatalf("ServeDNS = (%d, %v), want success", rcode, err)
	}
	if nextCalled {
		t.Fatal("challenge query was forwarded to the next plugin")
	}
	if len(recorder.Msg.Answer) != 1 {
		t.Fatalf("answer count = %d, want 1", len(recorder.Msg.Answer))
	}
	txt := recorder.Msg.Answer[0].(*dns.TXT)
	if got, want := txt.Txt[0], challenge.DNS01KeyAuthorization(); got != want {
		t.Fatalf("TXT value = %q, want %q", got, want)
	}

	if err := solver.CleanUp(context.Background(), challenge); err != nil {
		t.Fatal(err)
	}
	if got := len(solver.values(request.Question[0].Name)); got != 1 {
		t.Fatalf("record count after first cleanup = %d, want 1", got)
	}
	if err := solver.CleanUp(context.Background(), challenge); err != nil {
		t.Fatal(err)
	}
	recorder = dnstest.NewRecorder(&test.ResponseWriter{})
	rcode, err = handler.ServeDNS(context.Background(), recorder, request)
	if err != nil || rcode != dns.RcodeNameError || !nextCalled {
		t.Fatalf("ServeDNS after cleanup = (%d, %v), nextCalled=%v", rcode, err, nextCalled)
	}
}

type fakeCertificateManager struct {
	loadCalls   int
	manageCalls int
	ctx         context.Context
	certificate *ctls.Certificate
	loadErr     error
	manageErr   error
}

func (m *fakeCertificateManager) LoadManaged(context.Context, []string) error {
	m.loadCalls++
	return m.loadErr
}

func (m *fakeCertificateManager) ManageAsync(ctx context.Context, _ []string) error {
	m.manageCalls++
	m.ctx = ctx
	return m.manageErr
}

func (m *fakeCertificateManager) GetCertificate(*ctls.ClientHelloInfo) (*ctls.Certificate, error) {
	return m.certificate, nil
}

type fakeACMEBackend struct {
	managers map[acmeConfigKey]certificateManager
	stops    int
}

func (b *fakeACMEBackend) Manager(key acmeConfigKey) certificateManager { return b.managers[key] }
func (b *fakeACMEBackend) Stop()                                        { b.stops++ }

func TestACMERuntimeLifecycle(t *testing.T) {
	options := acmeOptions{
		domains: []string{"dns.example"},
		ca:      "https://ca.example/directory",
		storage: t.TempDir(),
	}
	manager := &fakeCertificateManager{certificate: &ctls.Certificate{}}
	backend := &fakeACMEBackend{}
	runtime := newACMERuntime(func(entries []*acmeEntry, _ *acmeDNS01Solver) (acmeBackend, error) {
		backend.managers = make(map[acmeConfigKey]certificateManager, len(entries))
		for _, entry := range entries {
			backend.managers[entry.key] = manager
		}
		return backend, nil
	})
	entry, err := runtime.add(options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.getCertificate(&ctls.ClientHelloInfo{}); !errors.Is(err, errACMENotReady) {
		t.Fatalf("pre-start GetCertificate error = %v", err)
	}
	if err := runtime.start(); err != nil {
		t.Fatal(err)
	}
	if manager.loadCalls != 1 {
		t.Fatalf("LoadManaged calls = %d, want 1", manager.loadCalls)
	}
	if manager.manageCalls != 1 {
		t.Fatalf("ManageAsync calls = %d, want 1", manager.manageCalls)
	}
	if cert, err := entry.getCertificate(&ctls.ClientHelloInfo{}); err != nil || cert != manager.certificate {
		t.Fatalf("GetCertificate = (%p, %v), want %p", cert, err, manager.certificate)
	}
	if _, err := entry.getCertificate(&ctls.ClientHelloInfo{ServerName: "other.example"}); !errors.Is(err, errACMENameNotManaged) {
		t.Fatalf("unmanaged GetCertificate error = %v, want %v", err, errACMENameNotManaged)
	}
	if err := runtime.stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-manager.ctx.Done():
	default:
		t.Fatal("management context was not canceled")
	}
	if err := runtime.stop(); err != nil {
		t.Fatal(err)
	}
	if backend.stops != 1 {
		t.Fatalf("backend stops = %d, want 1", backend.stops)
	}
}

func TestACMEEntryManagedNames(t *testing.T) {
	entry := &acmeEntry{options: acmeOptions{domains: []string{"dns.example", "*.wild.example"}}}
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"DNS.EXAMPLE.", true},
		{"one.wild.example", true},
		{"two.one.wild.example", false},
		{"other.example", false},
	} {
		if got := entry.manages(tc.name); got != tc.want {
			t.Errorf("manages(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestACMERuntimeDeduplicatesAndRejectsConflicts(t *testing.T) {
	runtime := newACMERuntime(nil)
	first := acmeOptions{domains: []string{"a.example", "b.example"}, ca: "https://ca.example", storage: "one"}
	entry, err := runtime.add(first)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := runtime.add(acmeOptions{domains: []string{"b.example", "a.example"}, ca: first.ca, storage: first.storage})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate != entry {
		t.Fatal("equivalent ACME configurations were not deduplicated")
	}
	_, err = runtime.add(acmeOptions{domains: []string{"a.example"}, ca: "https://other-ca.example", storage: "two"})
	if err == nil || !strings.Contains(err.Error(), "conflicting options") {
		t.Fatalf("conflicting configuration error = %v", err)
	}
}

func TestACMERuntimeStartupFailures(t *testing.T) {
	options := acmeOptions{domains: []string{"dns.example"}, ca: "https://ca.example", storage: t.TempDir()}
	for _, tc := range []struct {
		name      string
		manager   *fakeCertificateManager
		available bool
	}{
		{"load failure releases backend", &fakeCertificateManager{loadErr: errors.New("load failed")}, false},
		{"management failure keeps loaded certificate", &fakeCertificateManager{certificate: &ctls.Certificate{}, manageErr: errors.New("manage failed")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &fakeACMEBackend{}
			runtime := newACMERuntime(func(entries []*acmeEntry, _ *acmeDNS01Solver) (acmeBackend, error) {
				backend.managers = map[acmeConfigKey]certificateManager{entries[0].key: tc.manager}
				return backend, nil
			})
			entry, err := runtime.add(options)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.start(); err == nil {
				t.Fatal("startup succeeded, want error")
			}
			_, err = entry.getCertificate(&ctls.ClientHelloInfo{})
			if tc.available && err != nil {
				t.Fatalf("loaded certificate became unavailable: %v", err)
			}
			if !tc.available && !errors.Is(err, errACMENotReady) {
				t.Fatalf("GetCertificate error = %v, want %v", err, errACMENotReady)
			}
			if err := runtime.stop(); err != nil {
				t.Fatal(err)
			}
			if backend.stops != 1 {
				t.Fatalf("backend stops = %d, want 1", backend.stops)
			}
		})
	}
}
