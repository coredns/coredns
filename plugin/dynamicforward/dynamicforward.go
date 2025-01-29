package dynamicforward

import (
	"context"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/forward"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/miekg/dns"
	"log"
	"sync"
)

// DynamicForward main struct of plugin
type DynamicForward struct {
	Next        plugin.Handler
	Namespace   string
	ServiceName string
	forwardTo   []string
	mu          sync.RWMutex
	forwarder   *forward.Forward
	options     *forward.Options
	cond        *sync.Cond
}

func (df *DynamicForward) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	//wait forwarder
	df.cond.L.Lock()

	if df.forwarder == nil {
		df.cond.Wait()
	}

	df.cond.L.Unlock()

	return df.forwarder.ServeDNS(ctx, w, r)
}

// UpdateForwardServers update list servers for forward requests
func (df *DynamicForward) UpdateForwardServers(newServers []string, config DynamicForwardConfig) {
	//df.mu.Lock()
	//defer df.mu.Unlock()
	df.cond.L.Lock()

	// Clear proxy list `Forwarder`
	df.forwarder = forward.New()
	// Fill up list servers
	df.forwardTo = newServers

	// Create new list proxy
	for _, server := range newServers {
		// Create proxy
		proxyInstance := proxy.NewProxy(server, server, transport.DNS)
		proxyInstance.SetExpire(config.Expire)
		proxyInstance.SetReadTimeout(config.HealthCheck)

		df.forwarder.SetProxy(proxyInstance)
	}

	df.cond.Broadcast()
	df.cond.L.Unlock()

	log.Printf("[dynamicforward] Forward servers updated: %v", newServers)
}

// Name return plugin name
func (df *DynamicForward) Name() string { return "dynamicforward" }
