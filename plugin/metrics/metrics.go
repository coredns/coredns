// Package metrics implement a handler and plugin that provides Prometheus metrics.
package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/reuseport"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/prometheus/exporter-toolkit/web"
)

// logAdapter adapts our existing logging to the slog.Logger interface
type logAdapter struct{}

func (l *logAdapter) Info(msg string, args ...any) {
	log.Infof(msg, args...)
}

func (l *logAdapter) Error(msg string, args ...any) {
	log.Errorf(msg, args...)
}

func (l *logAdapter) Debug(msg string, args ...any) {
	log.Debugf(msg, args...)
}

func (l *logAdapter) Warn(msg string, args ...any) {
	log.Warningf(msg, args...)
}

func (l *logAdapter) With(args ...any) *slog.Logger {
	return slog.Default()
}

func (l *logAdapter) WithGroup(name string) slog.Handler {
	// Since our logging system doesn't support groups, we'll just return the same handler
	return l
}

func (l *logAdapter) Log(ctx context.Context, level slog.Level, msg string, args ...any) {
	switch level {
	case slog.LevelDebug:
		l.Debug(msg, args...)
	case slog.LevelInfo:
		l.Info(msg, args...)
	case slog.LevelWarn:
		l.Warn(msg, args...)
	case slog.LevelError:
		l.Error(msg, args...)
	}
}

func (l *logAdapter) Enabled(ctx context.Context, level slog.Level) bool {
	// We'll consider all levels enabled since we want to pass through all logs
	return true
}

func (l *logAdapter) Handle(ctx context.Context, r slog.Record) error {
	// Convert the record to our logging format
	msg := r.Message
	args := make([]interface{}, 0, r.NumAttrs())
	r.Attrs(func(attr slog.Attr) bool {
		args = append(args, attr.Key, attr.Value)
		return true
	})

	switch r.Level {
	case slog.LevelDebug:
		l.Debug(msg, args...)
	case slog.LevelInfo:
		l.Info(msg, args...)
	case slog.LevelWarn:
		l.Warn(msg, args...)
	case slog.LevelError:
		l.Error(msg, args...)
	}
	return nil
}

func (l *logAdapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Create a new adapter with the additional attributes
	// Since our logging system doesn't support structured logging, we'll just return the same handler
	return l
}

func (l *logAdapter) Handler() slog.Handler {
	return l
}

func newLoggerAdapter() *slog.Logger {
	return slog.New(&logAdapter{})
}

// Metrics holds the prometheus configuration. The metrics' path is fixed to be /metrics .
type Metrics struct {
	Next plugin.Handler
	Addr string
	Reg  *prometheus.Registry

	ln      net.Listener
	lnSetup bool

	mux *http.ServeMux
	srv *http.Server

	zoneNames []string
	zoneMap   map[string]struct{}
	zoneMu    sync.RWMutex

	plugins map[string]struct{} // all available plugins, used to determine which plugin made the client write

	tlsConfig *tlsConfig
}

// tlsConfig is the TLS configuration for Metrics
type tlsConfig struct {
	// enabled controls whether TLS is active
	// Optional: Defaults to true when tls block is present
	enabled bool

	// certFile is the path to the server's certificate file in PEM format
	// Required when TLS is enabled
	certFile string

	// keyFile is the path to the server's private key file in PEM format
	// Required when TLS is enabled
	keyFile string

	// clientCAFile is the path to the CA certificate file for client verification
	// Optional: Only needed for client authentication
	// Default: No client verification
	// Can contain multiple CA certificates in a single PEM file
	clientCAFile string

	// minVersion is the minimum TLS version to accept
	// Optional: Defaults to tls.VersionTLS13
	// Possible values: tls.VersionTLS10 through tls.VersionTLS13
	minVersion uint16

	// clientAuthType controls how client certificates are handled
	// Optional: Defaults to "RequestClientCert" (strictest)
	// Possible values:
	//   - "RequestClientCert"
	//   - "RequireAnyClientCert"
	//   - "VerifyClientCertIfGiven"
	//   - "RequireAndVerifyClientCert"
	//   - "NoClientCert"
	clientAuthType string

	cancelFunc context.CancelFunc
}

// New returns a new instance of Metrics with the given address.
func New(addr string) *Metrics {
	met := &Metrics{
		Addr:      addr,
		Reg:       prometheus.DefaultRegisterer.(*prometheus.Registry),
		zoneMap:   make(map[string]struct{}),
		plugins:   pluginList(caddy.ListPlugins()),
		tlsConfig: nil,
	}

	return met
}

// MustRegister wraps m.Reg.MustRegister.
func (m *Metrics) MustRegister(c prometheus.Collector) {
	err := m.Reg.Register(c)
	if err != nil {
		// ignore any duplicate error, but fatal on any other kind of error
		if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
			log.Fatalf("Cannot register metrics collector: %s", err)
		}
	}
}

// AddZone adds zone z to m.
func (m *Metrics) AddZone(z string) {
	m.zoneMu.Lock()
	m.zoneMap[z] = struct{}{}
	m.zoneNames = keys(m.zoneMap)
	m.zoneMu.Unlock()
}

// RemoveZone remove zone z from m.
func (m *Metrics) RemoveZone(z string) {
	m.zoneMu.Lock()
	delete(m.zoneMap, z)
	m.zoneNames = keys(m.zoneMap)
	m.zoneMu.Unlock()
}

// ZoneNames returns the zones of m.
func (m *Metrics) ZoneNames() []string {
	m.zoneMu.RLock()
	s := m.zoneNames
	m.zoneMu.RUnlock()
	return s
}

// validate validates the Metrics configuration
func (m *Metrics) validate() error {
	if m.tlsConfig != nil && m.tlsConfig.enabled {
		if m.tlsConfig.certFile == "" {
			return fmt.Errorf("TLS enabled but no certificate file specified")
		}
		if m.tlsConfig.keyFile == "" {
			return fmt.Errorf("TLS enabled but no key file specified")
		}
		// Check if files exist
		if _, err := os.Stat(m.tlsConfig.certFile); err != nil {
			return fmt.Errorf("certificate file not found: %s", m.tlsConfig.certFile)
		}
		if _, err := os.Stat(m.tlsConfig.keyFile); err != nil {
			return fmt.Errorf("key file not found: %s", m.tlsConfig.keyFile)
		}
	}
	return nil
}

// OnStartup sets up the metrics on startup.
func (m *Metrics) OnStartup() error {
	if err := m.validate(); err != nil {
		return err
	}

	ln, err := reuseport.Listen("tcp", m.Addr)
	if err != nil {
		log.Errorf("Failed to start metrics handler: %s", err)
		return err
	}

	m.ln = ln
	m.lnSetup = true

	m.mux = http.NewServeMux()
	m.mux.Handle("/metrics", promhttp.HandlerFor(m.Reg, promhttp.HandlerOpts{}))

	server := &http.Server{
		Addr:    m.Addr,
		Handler: m.mux,
	}
	// Create server without TLS based on configuration
	tlsEnabled := m.tlsConfig != nil && m.tlsConfig.enabled
	m.srv = server

	if !tlsEnabled {
		go func() {
			if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Errorf("Failed to start HTTP metrics server: %s", err)
			}
		}()
		ListenAddr = ln.Addr().String() // For tests.
		return nil
	}

	// Create web config for ListenAndServe
	webConfig := &web.FlagConfig{
		WebListenAddresses: &[]string{m.Addr},
		WebSystemdSocket:   new(bool), // false by default
		WebConfigFile:      new(string),
	}

	// Create temporary YAML config file for TLS settings
	tmpFile, err := os.CreateTemp("", "metrics-tls-config-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temporary TLS config file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	yamlConfig := fmt.Sprintf(`
tls_server_config:
  cert_file: %s
  key_file: %s
  client_ca_file: %s
  client_auth_type: %s
  min_version: %d
`, m.tlsConfig.certFile, m.tlsConfig.keyFile, m.tlsConfig.clientCAFile, m.tlsConfig.clientAuthType, m.tlsConfig.minVersion)

	if _, err := tmpFile.WriteString(yamlConfig); err != nil {
		return fmt.Errorf("failed to write TLS config to temporary file: %w", err)
	}

	*webConfig.WebConfigFile = tmpFile.Name()

	// Create logger
	logger := newLoggerAdapter()

	// Create channel to signal when server is ready
	ready := make(chan bool)
	errChan := make(chan error)

	go func() {
		// Signal when server is ready
		close(ready)
		err := web.Serve(m.ln, server, webConfig, logger)
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("Failed to start HTTPS metrics server: %v", err)
			errChan <- err
		}
	}()

	// Wait for server to be ready or error
	select {
	case <-ready:
		// Server started successfully
	case err := <-errChan:
		return err
	case <-time.After(2 * time.Second):
		return fmt.Errorf("timeout waiting for server to start")
	}

	ListenAddr = ln.Addr().String() // For tests.
	return nil
}

// OnRestart stops the listener on reload.
func (m *Metrics) OnRestart() error {
	if !m.lnSetup {
		return nil
	}
	u.Unset(m.Addr)
	return m.stopServer()
}

func (m *Metrics) stopServer() error {
	if !m.lnSetup {
		return nil
	}
	// In tls case, just running the cancelFunc will signal shutdown
	if m.tlsConfig != nil && m.tlsConfig.cancelFunc != nil {
		m.tlsConfig.cancelFunc()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := m.srv.Shutdown(ctx); err != nil {
			log.Infof("Failed to stop prometheus http server: %s", err)
			return err
		}
	}
	m.lnSetup = false
	m.ln.Close()
	return nil
}

// OnFinalShutdown tears down the metrics listener on shutdown and restart.
func (m *Metrics) OnFinalShutdown() error { return m.stopServer() }

func keys(m map[string]struct{}) []string {
	sx := []string{}
	for k := range m {
		sx = append(sx, k)
	}
	return sx
}

// pluginList iterates over the returned plugin map from caddy and removes the "dns." prefix from them.
func pluginList(m map[string][]string) map[string]struct{} {
	pm := map[string]struct{}{}
	for _, p := range m["others"] {
		// only add 'dns.' plugins
		if len(p) > 3 {
			pm[p[4:]] = struct{}{}
			continue
		}
	}
	return pm
}

// ListenAddr is assigned the address of the prometheus listener. Its use is mainly in tests where
// we listen on "localhost:0" and need to retrieve the actual address.
var ListenAddr string

// shutdownTimeout is the maximum amount of time the metrics plugin will wait
// before erroring when it tries to close the metrics server
const shutdownTimeout time.Duration = time.Second * 5

var buildInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: plugin.Namespace,
	Name:      "build_info",
	Help:      "A metric with a constant '1' value labeled by version, revision, and goversion from which CoreDNS was built.",
}, []string{"version", "revision", "goversion"})
