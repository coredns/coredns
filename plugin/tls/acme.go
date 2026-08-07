package tls

import (
	"context"
	ctls "crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/caddyserver/certmagic"
	"github.com/mholt/acmez/v3/acme"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/net/idna"
)

var (
	errACMENotReady       = errors.New("ACME certificate is not ready")
	errACMENameNotManaged = errors.New("server name is not managed by this ACME configuration")
)

type acmeOptions struct {
	domains  []string
	email    string
	ca       string
	storage  string
	caRoot   string
	resolver string
}

type acmeConfigKey struct {
	domains  string
	email    string
	ca       string
	storage  string
	caRoot   string
	resolver string
}

func (o acmeOptions) key() acmeConfigKey {
	domains := append([]string(nil), o.domains...)
	sort.Strings(domains)
	return acmeConfigKey{
		domains:  strings.Join(domains, "\x00"),
		email:    o.email,
		ca:       o.ca,
		storage:  o.storage,
		caRoot:   o.caRoot,
		resolver: o.resolver,
	}
}

func defaultACMEOptions(root string) (acmeOptions, error) {
	if root == "" {
		root = "."
	}
	storage, err := filepath.Abs(filepath.Join(root, ".coredns", "acme"))
	if err != nil {
		return acmeOptions{}, fmt.Errorf("resolving ACME storage directory: %w", err)
	}
	return acmeOptions{ca: certmagic.DefaultACME.CA, storage: storage}, nil
}

func normalizeACMEDomain(domain string) (string, error) {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	wildcard := strings.HasPrefix(domain, "*.")
	check := strings.TrimPrefix(domain, "*.")
	if domain == "" || check == "" || strings.Contains(check, "*") || net.ParseIP(check) != nil {
		return "", fmt.Errorf("invalid ACME domain %q", domain)
	}
	check, err := idna.Lookup.ToASCII(check)
	if err != nil {
		return "", fmt.Errorf("invalid ACME domain %q: %w", domain, err)
	}
	if _, ok := dns.IsDomainName(check); !ok {
		return "", fmt.Errorf("invalid ACME domain %q", domain)
	}
	if wildcard {
		return "*." + check, nil
	}
	return check, nil
}

func validateACMEOptions(o *acmeOptions) error {
	if len(o.domains) == 0 {
		return errors.New("ACME requires at least one domain")
	}

	seen := make(map[string]struct{}, len(o.domains))
	domains := o.domains[:0]
	for _, domain := range o.domains {
		normalized, err := normalizeACMEDomain(domain)
		if err != nil {
			return err
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		domains = append(domains, normalized)
	}
	o.domains = domains

	u, err := url.Parse(o.ca)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("invalid ACME CA URL %q", o.ca)
	}
	if o.resolver != "" {
		host, port, err := net.SplitHostPort(o.resolver)
		if err != nil {
			return fmt.Errorf("invalid ACME resolver %q: %w", o.resolver, err)
		}
		if host == "" {
			return fmt.Errorf("invalid ACME resolver %q: host is empty", o.resolver)
		}
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return fmt.Errorf("invalid ACME resolver port %q", port)
		}
	}
	if _, err := loadACMETrustedRoots(o.caRoot); err != nil {
		return err
	}
	return nil
}

type acmeDNS01Solver struct {
	mu      sync.RWMutex
	records map[string]map[string]int
}

func newACMEDNS01Solver() *acmeDNS01Solver {
	return &acmeDNS01Solver{records: make(map[string]map[string]int)}
}

func (s *acmeDNS01Solver) Present(ctx context.Context, challenge acme.Challenge) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name := normalizeChallengeName(challenge.DNS01TXTRecordName())
	value := challenge.DNS01KeyAuthorization()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records[name] == nil {
		s.records[name] = make(map[string]int)
	}
	s.records[name][value]++
	return nil
}

func (s *acmeDNS01Solver) CleanUp(_ context.Context, challenge acme.Challenge) error {
	name := normalizeChallengeName(challenge.DNS01TXTRecordName())
	value := challenge.DNS01KeyAuthorization()

	s.mu.Lock()
	defer s.mu.Unlock()
	values := s.records[name]
	if values[value] <= 1 {
		delete(values, value)
	} else {
		values[value]--
	}
	if len(values) == 0 {
		delete(s.records, name)
	}
	return nil
}

func (s *acmeDNS01Solver) values(name string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := s.records[normalizeChallengeName(name)]
	answer := make([]string, 0, len(values))
	for value := range values {
		answer = append(answer, value)
	}
	sort.Strings(answer)
	return answer
}

func normalizeChallengeName(name string) string {
	return strings.ToLower(dns.Fqdn(name))
}

type acmeChallengeHandler struct {
	Next   plugin.Handler
	solver *acmeDNS01Solver
}

func (h *acmeChallengeHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if len(r.Question) != 1 || r.Question[0].Qtype != dns.TypeTXT || r.Question[0].Qclass != dns.ClassINET {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	question := r.Question[0]
	values := h.solver.values(question.Name)
	if len(values) == 0 {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	response := new(dns.Msg)
	response.SetReply(r)
	response.Authoritative = true
	for _, value := range values {
		response.Answer = append(response.Answer, &dns.TXT{
			Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET},
			Txt: []string{value},
		})
	}
	if err := w.WriteMsg(response); err != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil
}

func (*acmeChallengeHandler) Name() string { return "tls" }

type certificateManager interface {
	LoadManaged(context.Context, []string) error
	ManageAsync(context.Context, []string) error
	GetCertificate(*ctls.ClientHelloInfo) (*ctls.Certificate, error)
}

type acmeBackend interface {
	Manager(acmeConfigKey) certificateManager
	Stop()
}

type acmeBackendFactory func([]*acmeEntry, *acmeDNS01Solver) (acmeBackend, error)

type acmeEntry struct {
	options acmeOptions
	key     acmeConfigKey

	mu      sync.RWMutex
	manager certificateManager
}

func (e *acmeEntry) setManager(manager certificateManager) {
	e.mu.Lock()
	e.manager = manager
	e.mu.Unlock()
}

func (e *acmeEntry) getCertificate(hello *ctls.ClientHelloInfo) (*ctls.Certificate, error) {
	if hello != nil && hello.ServerName != "" && !e.manages(hello.ServerName) {
		return nil, fmt.Errorf("%w: %q", errACMENameNotManaged, hello.ServerName)
	}
	e.mu.RLock()
	manager := e.manager
	e.mu.RUnlock()
	if manager == nil {
		return nil, fmt.Errorf("%w for %q", errACMENotReady, e.options.domains)
	}
	return manager.GetCertificate(hello)
}

func (e *acmeEntry) manages(serverName string) bool {
	serverName, err := normalizeACMEDomain(serverName)
	if err != nil {
		return false
	}
	for _, domain := range e.options.domains {
		if certmagic.MatchWildcard(serverName, domain) {
			return true
		}
	}
	return false
}

func (e *acmeEntry) tlsConfig() *ctls.Config {
	return &ctls.Config{
		MinVersion:     ctls.VersionTLS12,
		GetCertificate: e.getCertificate,
	}
}

type acmeRuntime struct {
	mu sync.Mutex

	entries           map[acmeConfigKey]*acmeEntry
	domainOwners      map[string]acmeConfigKey
	solver            *acmeDNS01Solver
	backendFactory    acmeBackendFactory
	backend           acmeBackend
	cancel            context.CancelFunc
	handlersInstalled bool
	started           bool
	stopped           bool
}

func newACMERuntime(factory acmeBackendFactory) *acmeRuntime {
	return &acmeRuntime{
		entries:        make(map[acmeConfigKey]*acmeEntry),
		domainOwners:   make(map[string]acmeConfigKey),
		solver:         newACMEDNS01Solver(),
		backendFactory: factory,
	}
}

func (r *acmeRuntime) add(options acmeOptions) (*acmeEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return nil, errors.New("cannot add ACME configuration after startup")
	}

	key := options.key()
	if entry := r.entries[key]; entry != nil {
		return entry, nil
	}
	for _, domain := range options.domains {
		if owner, ok := r.domainOwners[domain]; ok && owner != key {
			return nil, fmt.Errorf("ACME domain %q is configured with conflicting options", domain)
		}
	}

	entry := &acmeEntry{options: options, key: key}
	r.entries[key] = entry
	for _, domain := range options.domains {
		r.domainOwners[domain] = key
	}
	return entry, nil
}

func (r *acmeRuntime) installChallengeHandlers(c *caddy.Controller) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handlersInstalled {
		return
	}
	r.handlersInstalled = true
	dnsserver.AddPluginToAllServerBlocks(c, func(next plugin.Handler) plugin.Handler {
		return &acmeChallengeHandler{Next: next, solver: r.solver}
	})
}

func (r *acmeRuntime) start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started || r.stopped {
		return nil
	}
	entries := make([]*acmeEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		entries = append(entries, entry)
	}
	backend, err := r.backendFactory(entries, r.solver)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	for _, entry := range entries {
		manager := backend.Manager(entry.key)
		if manager == nil {
			cancel()
			backend.Stop()
			for _, configured := range entries {
				configured.setManager(nil)
			}
			return fmt.Errorf("no ACME certificate manager for %q", entry.options.domains)
		}
		entry.setManager(manager)
		if err := manager.LoadManaged(ctx, entry.options.domains); err != nil {
			cancel()
			backend.Stop()
			for _, configured := range entries {
				configured.setManager(nil)
			}
			return fmt.Errorf("loading managed certificates for %q: %w", entry.options.domains, err)
		}
	}
	r.backend = backend
	r.cancel = cancel
	r.started = true
	var errs []error
	for _, entry := range entries {
		entry.mu.RLock()
		manager := entry.manager
		entry.mu.RUnlock()
		if err := manager.ManageAsync(ctx, entry.options.domains); err != nil {
			errs = append(errs, fmt.Errorf("starting ACME management for %q: %w", entry.options.domains, err))
		}
	}
	return errors.Join(errs...)
}

func (r *acmeRuntime) stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return nil
	}
	r.stopped = true
	if r.cancel != nil {
		r.cancel()
	}
	if r.backend != nil {
		r.backend.Stop()
	}
	return nil
}

type certmagicBackend struct {
	cache    *certmagic.Cache
	managers map[acmeConfigKey]*certmagicManager
}

type certmagicManager struct{ *certmagic.Config }

func (m *certmagicManager) LoadManaged(ctx context.Context, domains []string) error {
	for _, domain := range domains {
		if _, err := m.CacheManagedCertificate(ctx, domain); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func newCertmagicBackend(entries []*acmeEntry, solver *acmeDNS01Solver) (acmeBackend, error) {
	var configsMu sync.RWMutex
	configsByDomain := make(map[string]*certmagic.Config)
	logger := newACMELogger()
	cache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(cert certmagic.Certificate) (*certmagic.Config, error) {
			configsMu.RLock()
			defer configsMu.RUnlock()
			for _, name := range cert.Names {
				if cfg := configsByDomain[strings.ToLower(strings.TrimSuffix(name, "."))]; cfg != nil {
					return cfg, nil
				}
			}
			return nil, fmt.Errorf("no ACME configuration for certificate names %q", cert.Names)
		},
		Logger: logger,
	})

	backend := &certmagicBackend{cache: cache, managers: make(map[acmeConfigKey]*certmagicManager)}
	for _, entry := range entries {
		roots, err := loadACMETrustedRoots(entry.options.caRoot)
		if err != nil {
			cache.Stop()
			return nil, err
		}
		cfg := certmagic.New(cache, certmagic.Config{
			DefaultServerName: entry.options.domains[0],
			Storage:           &certmagic.FileStorage{Path: entry.options.storage},
			Logger:            logger,
		})
		issuer := certmagic.NewACMEIssuer(cfg, certmagic.ACMEIssuer{
			CA:                        entry.options.ca,
			Email:                     entry.options.email,
			Agreed:                    true,
			DisableHTTPChallenge:      true,
			DisableTLSALPNChallenge:   true,
			DisableDistributedSolvers: true,
			DNS01Solver:               solver,
			TrustedRoots:              roots,
			Resolver:                  entry.options.resolver,
			Logger:                    logger,
		})
		cfg.Issuers = []certmagic.Issuer{issuer}
		backend.managers[entry.key] = &certmagicManager{Config: cfg}

		configsMu.Lock()
		for _, domain := range entry.options.domains {
			configsByDomain[domain] = cfg
		}
		configsMu.Unlock()
	}
	return backend, nil
}

func (b *certmagicBackend) Manager(key acmeConfigKey) certificateManager { return b.managers[key] }
func (b *certmagicBackend) Stop()                                        { b.cache.Stop() }

func loadACMETrustedRoots(path string) (*x509.CertPool, error) {
	if path == "" {
		return nil, nil
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading ACME CA root %q: %w", path, err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("ACME CA root %q contains no certificates", path)
	}
	return roots, nil
}

func newACMELogger() *zap.Logger {
	encoder := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		MessageKey:  "message",
		NameKey:     "logger",
		EncodeName:  zapcore.FullNameEncoder,
		LineEnding:  zapcore.DefaultLineEnding,
		EncodeLevel: zapcore.LowercaseLevelEncoder,
	})
	cores := make([]zapcore.Core, 0, 4)
	for _, level := range []zapcore.Level{zapcore.DebugLevel, zapcore.InfoLevel, zapcore.WarnLevel, zapcore.ErrorLevel} {
		selected := level
		cores = append(cores, zapcore.NewCore(
			encoder,
			zapcore.AddSync(acmeLogWriter{level: selected}),
			zap.LevelEnablerFunc(func(candidate zapcore.Level) bool {
				if selected == zapcore.ErrorLevel {
					return candidate >= selected
				}
				return candidate == selected
			}),
		))
	}
	return zap.New(zapcore.NewTee(cores...))
}

type acmeLogWriter struct{ level zapcore.Level }

func (w acmeLogWriter) Write(message []byte) (int, error) {
	text := strings.TrimSpace(string(message))
	switch w.level {
	case zapcore.DebugLevel:
		log.Debug(text)
	case zapcore.InfoLevel:
		log.Info(text)
	case zapcore.WarnLevel:
		log.Warning(text)
	default:
		log.Error(text)
	}
	return len(message), nil
}

func (acmeLogWriter) Sync() error { return nil }

type acmeRuntimeStorageKey struct{}

func getACMERuntime(c *caddy.Controller) *acmeRuntime {
	key := acmeRuntimeStorageKey{}
	if value := c.Get(key); value != nil {
		return value.(*acmeRuntime)
	}
	runtime := newACMERuntime(newCertmagicBackend)
	c.Set(key, runtime)
	c.OnShutdown(runtime.stop)
	return runtime
}

func acmeStartupHook(event caddy.EventName, info any) error {
	if event != caddy.InstanceStartupEvent {
		return nil
	}
	instance, ok := info.(*caddy.Instance)
	if !ok {
		return fmt.Errorf("unexpected ACME startup event payload %T", info)
	}
	instance.StorageMu.RLock()
	value := instance.Storage[acmeRuntimeStorageKey{}]
	instance.StorageMu.RUnlock()
	if value == nil {
		return nil
	}
	return value.(*acmeRuntime).start()
}
