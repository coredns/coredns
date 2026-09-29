package dso

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"os"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"weak"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/coredns/coredns/plugin/pkg/reuseport"

	"github.com/miekg/dns"
)

// DSO relies on CoreDNS configuration and some of the details are unknown during parsing.
// Therefore initialization is done in stages:
//   1. Collect configurations during parsing
//   2. Verify and resolve addresses during startup
//
// DSO runs its own servers that must be coordinated with CoreDNS. Graceful restarts are
// in particular troublesome. For DSO graceful means:
//   - Previously allocated listeners remain bound with new connections being queued
//   - Existing connections are served until new instance is installed and old instance is discarded
//
// CoreDNS offers various lifecycle callbacks. Normal callback flow looks like:
//   - OnStartup (inst0) -> OnShutdown (inst0)
//   - OnStartup (inst0) -> OnRestart (inst0) -> OnStartup (inst1) -> OnShutdown (inst0) -> OnShutdown (inst1)
//
// If restart fails, at any point, OnRestartFailed (inst0) is called to recover. Recovery is not always possible
// and there are quite a few failure states to handle:
//   1. OnRestart (inst0) failure by downstream: recovery
//   2. OnStartup (inst1) failure by upstream: recovery
//   3. OnStartup (inst1) failure by downstream: recovery
//   4. OnShutdown (inst0) failure by upstream: shutdown
//
// Upon successful restart old listeners with corresponding servers shut down. Upon recovery old listeners are repaired
// and service resumed.

type (
	// processDSO is per-process DSO handler.
	processDSO struct {
		procMu    sync.Mutex
		handlers  map[weak.Pointer[instanceDSO]]struct{}
		listeners map[string]*listener
	}

	// instanceDSO is per-instance DSO handler.
	instanceDSO struct {
		*processDSO

		configs  configSet
		starters map[string]*serverStarter

		instanceFinalizer runtime.Cleanup
	}

	// siteDSO is per-site DSO handler.
	siteDSO struct {
		*instanceDSO

		Next plugin.Handler
	}

	// serverStarter starts server after it's fully configured.
	serverStarter struct {
		config *Config
		state  atomic.Uint32

		mu             sync.Mutex
		addrs          [2]string    // {0: tcp, 1: tls}
		listeners      [2]*listener // only change when paused
		listenersGroup sync.WaitGroup
		upstream       *dnsserver.Server
		server         *Server
	}

	// listener pairs [net.TCPListener] with its corresponding [net.Listener.Accept] error, if any.
	listener struct {
		*net.TCPListener
		err error
	}

	// configSet is dns <-> dso pairs as parsed from Corefile.
	configSet map[*dnsserver.Config]*Config

	// instanceDSOKey is key to get [instanceDSO] from [caddy.Instance.Storage].
	instanceDSOKey struct{}
)

var globalDSO = processDSO{}

// Servers returns DSO servers in [caddy.Instance].
func Servers(inst *caddy.Instance) (servers []*Server) {
	inst.StorageMu.RLock()
	handler, ok := inst.Storage[instanceDSOKey{}]
	inst.StorageMu.RUnlock()

	if ok {
		handler := handler.(*instanceDSO)

		handler.procMu.Lock()
		defer handler.procMu.Unlock()

		for _, s := range handler.starters {
			s.mu.Lock()
			if s.server != nil {
				servers = append(servers, s.server)
			}
			s.mu.Unlock()
		}
	}
	return servers
}

var hookOnce sync.Once

// getInstanceDSO finds existing or allocates new per-instance DSO handler.
func (h *processDSO) getInstanceDSO(c *caddy.Controller) *instanceDSO {
	instH, ok := c.Get(instanceDSOKey{}).(*instanceDSO)
	if !ok {
		h.procMu.Lock()
		defer h.procMu.Unlock()

		instH = &instanceDSO{processDSO: h}
		c.Set(instanceDSOKey{}, instH)
		c.OnStartup(instH.onStartup)
		c.OnRestart(instH.onRestart)
		c.OnRestartFailed(instH.onRestartFailed)
		c.OnShutdown(instH.onShutdown)
	}
	hookOnce.Do(func() {
		caddy.RegisterEventHook(Name, func(event caddy.EventName, info interface{}) error {
			if event != caddy.InstanceStartupEvent {
				return nil
			}
			h.onInstanceStarted()
			return nil
		})
	})
	return instH
}

// getListener finds existing or allocates new listener.
//
// Broken listeners are repaired.
func (h *processDSO) getListener(addr string) (ln *listener, err error) {
	if h.listeners == nil {
		h.listeners = make(map[string]*listener)
	}
	ln, ok := h.listeners[addr]
	if ok && ln.resume() == nil {
		return ln, nil
	}
	lnTCP, err := reuseport.Listen("tcp", addr)
	if err != nil {
		return ln, err
	}
	if !ok {
		ln = &listener{}
		h.listeners[addr] = ln
	}
	ln.TCPListener, ln.err = lnTCP.(*net.TCPListener), nil
	return ln, nil
}

func (h *processDSO) onInstanceStarted() {
	h.procMu.Lock()
	defer h.procMu.Unlock()

	h.cleanup()
}

func (h *processDSO) onInstanceFinalizer(instH *instanceDSO) {
	h.procMu.Lock()
	defer h.procMu.Unlock()

	instH.discard()
	h.cleanup()
}

// cleanup removes unreachable listeners and handlers.
func (h *processDSO) cleanup() {
	var seen []*listener
	maps.DeleteFunc(h.handlers, func(weakInstH weak.Pointer[instanceDSO], _ struct{}) bool {
		instH := weakInstH.Value()
		if instH == nil {
			return true
		}
		for _, starter := range instH.starters {
			for _, ln := range starter.listeners {
				if ln != nil {
					seen = append(seen, ln)
				}
			}
		}
		return false
	})
	maps.DeleteFunc(h.listeners, func(_ string, ln *listener) bool {
		if !slices.Contains(seen, ln) {
			ln.Close()
			return true
		}
		return false
	})
}

func (h *instanceDSO) newSiteDSO() *siteDSO {
	return &siteDSO{instanceDSO: h}
}

func (h *instanceDSO) onStartup() (err error) {
	h.procMu.Lock()
	defer h.procMu.Unlock()

	if h.starters, err = h.configs.resolve(); err != nil {
		return plugin.Error(Name, err)
	}
	if err = h.resume(); err != nil {
		h.cleanup()
		return plugin.Error(Name, err)
	}
	h.track()
	return nil
}

func (h *instanceDSO) onRestart() error {
	h.procMu.Lock()
	defer h.procMu.Unlock()

	h.pause()
	return nil
}

func (h *instanceDSO) onRestartFailed() (err error) {
	h.procMu.Lock()
	defer h.procMu.Unlock()

	if !h.isPaused() {
		return nil
	}
	if !h.isOnline() {
		h.discard()
		h.cleanup()
		return nil
	}
	if err = h.resume(); err != nil {
		h.discard()
		h.cleanup()
		return plugin.Error(Name, err)
	}
	return nil
}

func (h *instanceDSO) onShutdown() error {
	h.procMu.Lock()
	defer h.procMu.Unlock()

	h.discard()
	h.cleanup()
	return nil
}

// isOnline checks whether handler's [caddy.Instance] was removed by CoreDNS.
func (h *instanceDSO) isOnline() (ok bool) {
	for _, inst := range caddy.Instances() {
		inst.StorageMu.RLock()
		ok = inst.Storage[instanceDSOKey{}] == h
		inst.StorageMu.RUnlock()
		if ok {
			return true
		}
	}
	return false
}

// isPaused checks whether handler's servers are paused.
func (h *instanceDSO) isPaused() bool {
	for _, s := range h.starters {
		return s.isPaused() // either all paused or none
	}
	return false
}

// addConfigPair adds config parsed during setup.
func (h *instanceDSO) addConfigPair(dnsCfg *dnsserver.Config, dsoCfg *Config) {
	h.configs.add(dnsCfg, dsoCfg)
}

// setUpstream sets upstream of matching server starter.
func (h *instanceDSO) setUpstream(u *dnsserver.Server) {
	_, dnsAddr := parse.Transport(u.Addr)
	if s, ok := h.starters[dnsAddr]; ok {
		s.setUpstream(u)
	}
}

func (h *instanceDSO) track() {
	if h.handlers == nil {
		h.handlers = make(map[weak.Pointer[instanceDSO]]struct{})
	}
	key := weak.Make(h)
	if _, ok := h.handlers[key]; ok {
		return
	}
	h.handlers[key] = struct{}{}

	instances := caddy.Instances()
	i := slices.IndexFunc(instances, func(inst *caddy.Instance) bool {
		inst.StorageMu.RLock()
		defer inst.StorageMu.RUnlock()
		return inst.Storage[instanceDSOKey{}] == h
	})
	if i < 0 {
		log.Debugf("Failed to register instance cleanup")
		return
	}
	procH := h.processDSO
	h.instanceFinalizer = runtime.AddCleanup(instances[i], func(instH *instanceDSO) {
		go procH.onInstanceFinalizer(instH)
	}, h)
}

func (h *instanceDSO) discard() {
	key := weak.Make(h)
	if _, ok := h.handlers[key]; !ok {
		return
	}
	delete(h.handlers, key)

	h.shutdown(len(h.handlers) == 0)
	h.instanceFinalizer.Stop()
}

// resume ensures every starter has functional listeners and signals to resume accepting connections.
//
// On error it may leave unused listeners in [processDSO.listeners]. Follow up with [processDSO.cleanup].
func (h *instanceDSO) resume() (err error) {
	for _, s := range h.starters {
		for i, addr := range s.addrs {
			if addr == "" {
				continue
			}
			ln, err := h.getListener(addr)
			if err != nil {
				return err
			}
			s.listeners[i] = ln
		}
	}
	for _, s := range h.starters {
		s.resume()
	}
	return nil
}

// pause stops accepting connections on all started servers.
func (h *instanceDSO) pause() {
	var wg sync.WaitGroup
	for _, s := range h.starters {
		wg.Go(func() {
			s.pause()
		})
	}
	wg.Wait()
}

// shutdown shuts down all started servers.
func (h *instanceDSO) shutdown(unloading bool) {
	var wg sync.WaitGroup
	for _, s := range h.starters {
		wg.Go(func() {
			if unloading {
				s.shutdown(s.config.ShutdownReconnectInterval)
			} else {
				s.shutdown(s.config.RestartReconnectInterval)
			}
		})
	}
	wg.Wait()
}

// ServeDNS implements [plugin.Handler.ServeDNS].
func (h *siteDSO) ServeDNS(ctx context.Context, w dns.ResponseWriter, m *dns.Msg) (rcode int, err error) {
	h.setUpstream(ctx.Value(dnsserver.Key{}).(*dnsserver.Server))
	return plugin.NextOrFailure(Name, h.Next, ctx, w, m)
}

// Name implements [plugin.Handler.Name].
func (h *siteDSO) Name() string { return Name }

const (
	stateUpstreamSet uint32 = 1 << iota
	stateResumed
)

// setUpstream sets upstream and starts serving, if ready.
func (s *serverStarter) setUpstream(u *dnsserver.Server) {
	if s.state.Load()&stateUpstreamSet != 0 {
		return // hot path
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	old := s.state.Or(stateUpstreamSet)
	if old&stateUpstreamSet != 0 {
		return
	}
	s.upstream = u

	if old != stateResumed {
		return
	}
	s.onReady()
}

// resume starts serving, if ready.
func (s *serverStarter) resume() {
	s.mu.Lock()
	defer s.mu.Unlock()

	old := s.state.Or(stateResumed)
	if old != stateUpstreamSet {
		return
	}
	s.onReady()
}

// pause unblocks listeners and stops serving. Existing connections are unaffected.
func (s *serverStarter) pause() {
	s.mu.Lock()
	defer s.mu.Unlock()

	old := s.state.And(^stateResumed)
	if old&stateResumed == 0 {
		return
	}

	for _, ln := range s.listeners {
		if ln != nil {
			ln.pause()
		}
	}
	s.listenersGroup.Wait()
}

func (s *serverStarter) isPaused() bool {
	return s.state.Load()&stateResumed == 0
}

// shutdown shuts down started server.
func (s *serverStarter) shutdown(reconnectInterval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server == nil {
		return
	}
	s.server.Shutdown(context.Background(), reconnectInterval)
	s.listenersGroup.Wait()
	s.server = nil
}

func (s *serverStarter) onReady() {
	if s.server == nil {
		s.server = newServer(s.config, s.upstream)
	}
	if ln := s.listeners[0]; ln != nil {
		s.listenersGroup.Go(func() {
			ln.err = s.server.Serve(ln)
		})
		log.Infof("Paired dso://%v to %v", ln.Addr(), s.server.Upstream.Addr)
	}
	if ln := s.listeners[1]; ln != nil {
		s.listenersGroup.Go(func() {
			ln.err = s.server.ServeTLS(ln)
		})
		log.Infof("Paired dso://%v to %v", ln.Addr(), s.server.Upstream.Addr)
	}
}

var aLongTimeAgo = time.Unix(1, 0)

func (ln *listener) resume() error {
	switch {
	case errors.Is(ln.err, os.ErrDeadlineExceeded): // graceful pause
		ln.err = nil
		fallthrough
	case ln.err == nil:
		ln.err = ln.SetDeadline(time.Time{})
		if ln.err == nil {
			return nil
		}
		fallthrough
	default:
		log.Infof("dso://%v: %v", ln.Addr(), ln.err)
		ln.Close()
		return ln.err
	}
}

func (ln *listener) pause() {
	ln.SetDeadline(aLongTimeAgo)
}

func (cs *configSet) add(dnsCfg *dnsserver.Config, dsoCfg *Config) {
	if *cs == nil {
		*cs = make(map[*dnsserver.Config]*Config)
	}
	(*cs)[dnsCfg] = dsoCfg
}

func resolveAddr(host, port string) (string, error) {
	raw := net.JoinHostPort(host, port)
	addr, err := net.ResolveTCPAddr("tcp", raw)
	if err != nil {
		return "", fmt.Errorf("%w %v: %w", errAddress, raw, err)
	}
	return addr.String(), nil
}

// resolve verifies config pairs and allocates [serverStarter] for each DNS endpoint.
func (cs configSet) resolve() (starters map[string]*serverStarter, err error) {
	allAddrs := make(map[string]struct{}, len(cs)) // detect DSO <-> DSO and DSO -> DNS overlaps
	dsoAddrs := make(map[string]struct{}, len(cs)) // detect DSO <- DNS overlaps
	starters = make(map[string]*serverStarter, len(cs))
	for dnsCfg, dsoCfg := range cs {
		for _, host := range dnsCfg.ListenHosts {
			dnsAddr, err := resolveAddr(host, dnsCfg.Port)
			if err != nil {
				return nil, err
			}
			if _, ok := dsoAddrs[dnsAddr]; ok {
				return nil, fmt.Errorf("%w %v: already in use", errAddress, dnsAddr)
			}
			allAddrs[dnsAddr] = struct{}{}

			if _, ok := starters[dnsAddr]; !ok {
				starters[dnsAddr] = &serverStarter{}
			}

			// Bare configs contribute nothing and must be followed up by proper definition.
			if dsoCfg == bareConfig {
				continue
			}

			// DNS endpoint can be defined across multiple configs. Ensure that each endpoint has at most one DSO handler.
			if starters[dnsAddr].config != nil {
				return nil, errRedefined
			}
			if dsoCfg.TLSPort > 0 && dsoCfg.TLSConfig == nil {
				if dnsCfg.TLSConfig == nil {
					return nil, errTLSConfig
				}
				dsoCfg.TLSConfig = dnsCfg.TLSConfig.Clone()
			}
			dsoCfg.TsigSecret = maps.Clone(dnsCfg.TsigSecret) // TODO: https://github.com/coredns/coredns/pull/8434
			starters[dnsAddr].config = dsoCfg

			for i, port := range []int{dsoCfg.TCPPort, dsoCfg.TLSPort} {
				if port <= 0 {
					continue
				}
				dsoAddr, err := resolveAddr(host, strconv.Itoa(port))
				if err != nil {
					return nil, err
				}
				if _, ok := allAddrs[dsoAddr]; ok {
					return nil, fmt.Errorf("%w %v: already in use", errAddress, dsoAddr)
				}
				dsoAddrs[dsoAddr] = struct{}{}
				allAddrs[dsoAddr] = struct{}{}

				starters[dnsAddr].addrs[i] = dsoAddr
			}
		}
	}
	for dnsAddr, s := range starters {
		if s.config == nil {
			return nil, fmt.Errorf("%w for %v", errBare, dnsAddr)
		}
	}
	return starters, nil
}
