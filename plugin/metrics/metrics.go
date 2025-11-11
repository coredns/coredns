// Package metrics implement a handler and plugin that provides Prometheus metrics.
package metrics

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/reuseport"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/exporter-toolkit/web"
)

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

	tlsConfigPath string
}

// New returns a new instance of Metrics with the given address.
func New(addr string) *Metrics {
	met := &Metrics{
		Addr:          addr,
		Reg:           prometheus.DefaultRegisterer.(*prometheus.Registry),
		zoneMap:       make(map[string]struct{}),
		plugins:       pluginList(caddy.ListPlugins()),
		tlsConfigPath: "",
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

// OnStartup sets up the metrics on startup.
func (m *Metrics) OnStartup() error {
	ln, err := reuseport.Listen("tcp", m.Addr)
	if err != nil {
		log.Errorf("Failed to start metrics handler: %s", err)
		return err
	}

	m.ln = ln
	m.lnSetup = true

	m.mux = http.NewServeMux()
	m.mux.Handle("/metrics", promhttp.HandlerFor(m.Reg, promhttp.HandlerOpts{}))

	// creating some helper variables to avoid data races on m.srv and m.ln
	server := &http.Server{
		Addr:         m.Addr,
		Handler:      m.mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  5 * time.Second,
	}
	m.srv = server

	if m.tlsConfigPath == "" {
		go func() {
			if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Errorf("Failed to start HTTP metrics server: %s", err)
			}
		}()
		ListenAddr = ln.Addr().String() // For tests.
		return nil
	}

	// Check TLS config file existence
	if _, err := os.Stat(m.tlsConfigPath); os.IsNotExist(err) {
		log.Errorf("TLS config file does not exist: %s", m.tlsConfigPath)
		return err
	}

	// Create web config for ListenAndServe
	webConfig := &web.FlagConfig{
		WebListenAddresses: &[]string{m.Addr},
		WebSystemdSocket:   new(bool), // false by default
		WebConfigFile:      &m.tlsConfigPath,
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// Create channels for synchronization
	startResult := make(chan error, 1)

	go func() {
		// Try to start the server and immediately report result
		err := web.Serve(m.ln, server, webConfig, logger)
		if err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start HTTPS metrics server", "error", err)
			startResult <- err
		}
		// If we get here without error, server is running
	}()

	// Wait for startup errors
	select {
	case err := <-startResult:
		return err
	case <-time.After(200 * time.Millisecond):
		// No immediate error, server likely started successfully
		// web.Serve() validates TLS config at startup
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
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := m.srv.Shutdown(ctx); err != nil {
		log.Infof("Failed to stop prometheus http server: %s", err)
		return err
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
