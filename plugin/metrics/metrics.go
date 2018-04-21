// Package metrics implement a handler and plugin that provides Prometheus metrics.
package metrics

import (
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"

	"fmt"

	"github.com/coredns/coredns/coremain"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics/vars"
	"github.com/coredns/coredns/plugin/pkg/listener"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the prometheus configuration. The metrics' path is fixed to be /metrics
type Metrics struct {
	Next plugin.Handler

	zoneNames []string
	zoneMap   map[string]bool
	zoneMu    sync.RWMutex
	lsn       *metricsListener
}

// metricsListener is the HTTP Server that will return the data on external request
// it may be shared by several Metrics objects, allowing a consolitation of all metrics on the same server
type metricsListener struct {
	Reg   *prometheus.Registry
	alloc listener.AllocationToken
	ln    net.Listener
	mux   *http.ServeMux
}

// New returns a new instance of Metrics with the given zones
func New(listener *metricsListener, zones []string) *Metrics {
	zMap := map[string]bool{}
	for _, z := range zones {
		zMap[z] = true
	}
	met := &Metrics{
		zoneMap:   zMap,
		zoneNames: keys(zMap),
		lsn:       listener,
	}

	return met
}

func keys(m map[string]bool) []string {
	sx := []string{}
	for k := range m {
		sx = append(sx, k)
	}
	return sx
}

// AddZone adds zone z to m.
func (m *Metrics) AddZone(z string) {
	m.zoneMu.Lock()
	m.zoneMap[z] = true
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

func newListener(alloc listener.AllocationToken) *metricsListener {
	met := &metricsListener{
		Reg:   prometheus.NewRegistry(),
		alloc: alloc,
	}

	// Add the default collectors
	met.MustRegister(prometheus.NewGoCollector())
	met.MustRegister(prometheus.NewProcessCollector(os.Getpid(), ""))

	// Add all of our collectors
	met.MustRegister(buildInfo)
	met.MustRegister(vars.RequestCount)
	met.MustRegister(vars.RequestDuration)
	met.MustRegister(vars.RequestSize)
	met.MustRegister(vars.RequestDo)
	met.MustRegister(vars.RequestType)
	met.MustRegister(vars.ResponseSize)
	met.MustRegister(vars.ResponseRcode)

	// Initialize metrics.
	buildInfo.WithLabelValues(coremain.CoreVersion, coremain.GitCommit, runtime.Version()).Set(1)

	return met
}

func (m *metricsListener) Name() string { return "prometheus" }

// MustRegister wraps m.Reg.MustRegister.
func (m *metricsListener) MustRegister(c prometheus.Collector) { m.Reg.MustRegister(c) }

// OnStartup sets up the HTTP Listener for all Metrics associated with this address.
func (m *metricsListener) OnStartup() error {
	ln, err := m.alloc.AllocateListener()
	if err != nil {
		log.Errorf("Failed to start metrics handler: %s", err)
		return err
	}

	m.ln = ln
	m.mux = http.NewServeMux()
	m.mux.Handle("/metrics", promhttp.HandlerFor(m.Reg, promhttp.HandlerOpts{}))

	go func() {
		http.Serve(m.ln, m.mux)
	}()
	return nil
}

//ListeningAddr return the listening addr. An error is thrown if listener is not listening yet
func (m *metricsListener) ListeningAddr() (string, error) {
	if m.ln == nil {
		return "", fmt.Errorf("Server not yet spawn up - address is unknown")
	}
	return m.ln.Addr().String(), nil
}

// OnShutdown tears down the metrics listener on shutdown and restart.
func (m *metricsListener) OnShutdown() error {
	// We allow prometheus statements in multiple Server Blocks, but only the first
	// will open the listener, for the rest they are all nil; guard against that.
	if m.ln != nil {
		return m.ln.Close()
	}
	return nil
}

var (
	buildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Name:      "build_info",
		Help:      "A metric with a constant '1' value labeled by version, revision, and goversion from which CoreDNS was built.",
	}, []string{"version", "revision", "goversion"})
)
