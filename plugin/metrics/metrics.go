// Package metrics implement a handler and plugin that provides Prometheus metrics.
package metrics

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"

	"github.com/coredns/coredns/coremain"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics/vars"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the prometheus configuration. The metrics' path is fixed to be /metrics
type Metrics struct {
	Next plugin.Handler
	Addr string
	Reg  *prometheus.Registry

	zoneNames []string
	zoneMap   map[string]bool
	zoneMu    sync.RWMutex
}

// New returns a new instance of Metrics with the given address
func New(addr string) *Metrics {
	return &Metrics{
		Addr:    addr,
		Reg:     sharedStateFor(addr).reg,
		zoneMap: make(map[string]bool),
	}
}

// MustRegister wraps m.Reg.MustRegister.
func (m *Metrics) MustRegister(c prometheus.Collector) {
	if err := m.Reg.Register(c); err != nil {
		log.Printf("[ERROR] registering metric: %s", err)
		if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
			panic(err)
		}
	}
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

// OnStartup sets up the metrics on startup.
func (m *Metrics) OnStartup() error {
	addr, err := sharedStates[m.Addr].listenAndServe(m.Addr)
	if err != nil {
		return err
	}
	ListenAddr = addr.String() // for tests
	return nil
}

// OnShutdown tears down the metrics on shutdown and restart.
func (m *Metrics) OnShutdown() error {
	// Note: kept for backward compatibility but does nothing now. The listeners
	// used to be managed by the Metrics values but now they are shared amongst
	// plugins that use the same address.
	return nil
}

func keys(m map[string]bool) []string {
	sx := []string{}
	for k := range m {
		sx = append(sx, k)
	}
	return sx
}

// ListenAddr is assigned the address of the prometheus listener. Its use is mainly in tests where
// we listen on "localhost:0" and need to retrieve the actual address.
var ListenAddr string

var (
	// Notes on using a global registry
	// --------------------------------
	//
	// CoreDNS currently uses a pattern of declaring metrics as global variables
	// and registering them to
	//	registry = prometheus.NewRegistry()

	buildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Name:      "build_info",
		Help:      "A metric with a constant '1' value labeled by version, revision, and goversion from which CoreDNS was built.",
	}, []string{"version", "revision", "goversion"})
)

type sharedState struct {
	lstn net.Listener
	mux  http.ServeMux
	reg  *prometheus.Registry
}

func sharedStateFor(addr string) *sharedState {
	state, ok := sharedStates[addr]
	if !ok {
		state = newSharedState()
		sharedStates[addr] = state
		log.Print("install shared metrics state at", addr)
	}
	return state
}

func newSharedState() *sharedState {
	r := prometheus.NewRegistry()
	s := &sharedState{reg: r}
	s.mux.Handle("/metrics", promhttp.HandlerFor(s.reg, promhttp.HandlerOpts{}))

	// Add the default collectors
	r.MustRegister(prometheus.NewGoCollector())
	r.MustRegister(prometheus.NewProcessCollector(os.Getpid(), ""))

	// Add all of our collectors
	r.MustRegister(buildInfo)
	r.MustRegister(vars.RequestCount)
	r.MustRegister(vars.RequestDuration)
	r.MustRegister(vars.RequestSize)
	r.MustRegister(vars.RequestDo)
	r.MustRegister(vars.RequestType)
	r.MustRegister(vars.ResponseSize)
	r.MustRegister(vars.ResponseRcode)

	// Initialize metrics.
	buildInfo.WithLabelValues(coremain.CoreVersion, coremain.GitCommit, runtime.Version()).Set(1)
	return s
}

func (s *sharedState) listenAndServe(addr string) (net.Addr, error) {
	if s.lstn == nil {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("failed to start metrics handler: %s", err)
		}
		go http.Serve(l, &s.mux)
		s.lstn = l
		log.Printf("[INFO] installing /metrics endpoint at %s", l.Addr())
	}
	return s.lstn.Addr(), nil
}

var sharedStates = make(map[string]*sharedState)
