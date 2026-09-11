package dso

import (
	"context"
	"maps"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/parse"

	"github.com/miekg/dns"
)

const (
	stateWaiting        = 0
	stateUpstreamReady  = 1 << 0
	stateListenersReady = 1 << 1
	stateServing        = stateUpstreamReady | stateListenersReady
)

type (
	// serverBuilder starts or shuts down server when necessary.
	//
	// Two concurrent processes:
	//  - Plugin lifecycle: OnStartup, OnRestart, OnRestartFailed, etc
	//  - Learning upstream server via ServeDNS
	//
	// Server is started only once upstream is known and plugin is ready.
	serverBuilder struct {
		config *Config

		state     atomic.Uint32
		listeners [2]net.Listener // 0: tcp, 1: tls
		upstream  *dnsserver.Server
		server    *Server
	}

	instanceDSO struct {
		configs  map[*dnsserver.Config]*Config // dns <-> dso as parsed from Corefile
		builders map[string]*serverBuilder     // builders per dns addr
	}

	dso struct {
		*instanceDSO

		Next plugin.Handler
	}
)

func (b *serverBuilder) setUpstream(u *dnsserver.Server) {
	for s := b.state.Load(); s&stateUpstreamReady == 0; s = b.state.Load() {
		s1 := s | stateUpstreamReady
		if b.state.CompareAndSwap(s, s1) {
			b.upstream = u
			if s1 == stateServing {
				b.start()
			}
			break
		}
	}
}

func (b *serverBuilder) setListeners(lns [2]net.Listener) {
	for s := b.state.Load(); s&stateListenersReady == 0; s = b.state.Load() {
		s1 := s | stateListenersReady
		if b.state.CompareAndSwap(s, s1) {
			b.listeners = lns
			if s1 == stateServing {
				b.start()
			}
			break
		}
	}
}

func (b *serverBuilder) unsetListeners(reconnectInterval time.Duration) {
	for s := b.state.Load(); s&stateListenersReady != 0; s = b.state.Load() {
		s1 := s &^ stateListenersReady
		if b.state.CompareAndSwap(s, s1) {
			b.shutdown(reconnectInterval)
			break
		}
	}
}

func (b *serverBuilder) start() {
	b.server = newServer(b.config, b.upstream)
	if ln := b.listeners[0]; ln != nil {
		go b.server.Serve(ln)
		log.Infof("Paired dso://%v to %v", ln.Addr(), b.upstream.Addr)
	}
	if ln := b.listeners[1]; ln != nil {
		go b.server.ServeTLS(ln)
		log.Infof("Paired dso://%v to %v", ln.Addr(), b.upstream.Addr)
	}
}

func (b *serverBuilder) shutdown(reconnectInterval time.Duration) {
	if b.server != nil {
		b.server.Shutdown(context.Background(), reconnectInterval)
		b.server = nil
	} else {
		for i, ln := range b.listeners {
			if ln != nil {
				ln.Close()
				b.listeners[i] = nil
			}
		}
	}
}

func (h *instanceDSO) addConfig(dnsCfg *dnsserver.Config, dsoCfg *Config) {
	if h.configs == nil {
		h.configs = make(map[*dnsserver.Config]*Config)
	}
	h.configs[dnsCfg] = dsoCfg
}

func (h *instanceDSO) applyConfig() (err error) {
	// One DNS server can be defined by multiple server blocks in Corefile.
	// Ensure that at most one DSO definition exists per server.
	builders := make(map[string]*serverBuilder)
	for dnsCfg, dsoCfg := range h.configs {
		for _, h := range dnsCfg.ListenHosts {
			dnsAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(h, dnsCfg.Port))
			if err != nil {
				return err
			}
			dnsAddrStr := dnsAddr.String()
			if _, ok := builders[dnsAddrStr]; ok {
				return errRedefined
			}
			dsoCfg.TsigSecret = maps.Clone(dnsCfg.TsigSecret) // TODO: https://github.com/coredns/coredns/pull/8434
			dsoCfg.TLSConfig = dnsCfg.TLSConfig.Clone()
			builders[dnsAddrStr] = &serverBuilder{
				config: dsoCfg,
			}
		}
	}
	h.builders = builders
	h.configs = nil
	return nil
}

func (h *instanceDSO) setUpstream(u *dnsserver.Server) {
	_, dnsAddr := parse.Transport(u.Addr)
	if b, ok := h.builders[dnsAddr]; ok {
		b.setUpstream(u)
	}
}

func (h *instanceDSO) setListeners() (err error) {
	listeners := make(map[string][2]net.Listener, len(h.builders))
	defer func() {
		if err != nil {
			for _, lns := range listeners {
				for _, ln := range lns {
					if ln != nil {
						ln.Close()
					}
				}
			}
		}
	}()
	for dnsAddr, builder := range h.builders {
		h, _, err := net.SplitHostPort(dnsAddr)
		if err != nil {
			return err
		}
		var lns [2]net.Listener
		for i, p := range []int{builder.config.TCPPort, builder.config.TLSPort} {
			if p <= 0 {
				continue
			}
			dsoAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(h, strconv.Itoa(p)))
			if err != nil {
				return err
			}
			lns[i], err = net.ListenTCP("tcp", dsoAddr)
			if err != nil {
				return err
			}
		}
		listeners[dnsAddr] = lns
	}
	// Only proceed if all configured listeners were bound.
	for dnsAddr, lns := range listeners {
		h.builders[dnsAddr].setListeners(lns)
	}
	return nil
}

func (h *instanceDSO) unsetListeners(reconnectIntervalFunc func(cfg *Config) time.Duration) {
	var wg sync.WaitGroup
	for _, b := range h.builders {
		wg.Go(func() {
			b.unsetListeners(reconnectIntervalFunc(b.config))
		})
	}
	wg.Wait()
}

func (h *instanceDSO) onStartup() (err error) {
	if err = h.applyConfig(); err != nil {
		return plugin.Error(Name, err)
	}
	if err = h.setListeners(); err != nil {
		return plugin.Error(Name, err)
	}
	return nil
}

func (h *instanceDSO) onRestart() (err error) {
	h.unsetListeners(func(cfg *Config) time.Duration { return cfg.RestartReconnectInterval })
	return nil
}

func (h *instanceDSO) onRestartFailed() (err error) {
	h.setListeners()
	return nil
}

func (h *instanceDSO) onFinalShutdown() (err error) {
	h.unsetListeners(func(cfg *Config) time.Duration { return cfg.ShutdownReconnectInterval })
	return nil
}

// ServeDNS implements [plugin.Handler.ServeDNS].
func (h *dso) ServeDNS(ctx context.Context, w dns.ResponseWriter, m *dns.Msg) (rcode int, err error) {
	h.setUpstream(ctx.Value(dnsserver.Key{}).(*dnsserver.Server))
	return plugin.NextOrFailure(Name, h.Next, ctx, w, m)
}

// Name implements [plugin.Handler.Name].
func (h *dso) Name() string { return Name }

// Servers returns DSO servers in [caddy.Instance].
func Servers(inst *caddy.Instance) (servers []*Server) {
	if handler, ok := inst.Storage[instanceDSOKey{}]; ok {
		for _, b := range handler.(*instanceDSO).builders {
			if b.server != nil {
				servers = append(servers, b.server)
			}
		}
	}
	return servers
}
