// Package forward implements a forwarding proxy. It caches an upstream net.Conn for some time, so if the same
// client returns the upstream's Conn will be precached. Depending on how you benchmark this looks to be
// 50% faster than just opening a new connection for every client. It works with UDP and TCP and uses
// inband healthchecking.
package forward

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/debug"
	"github.com/coredns/coredns/plugin/dnstap"
	"github.com/coredns/coredns/plugin/metadata"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	ot "github.com/opentracing/opentracing-go"
	otext "github.com/opentracing/opentracing-go/ext"
)

var log = clog.NewWithPlugin("forward")

// Forward represents a plugin instance that can proxy requests to another (DNS) server. It has a list
// of proxies each representing one upstream proxy.
type Forward struct {
	concurrent int64 // atomic counters need to be first in struct for proper alignment

	proxies    []*Proxy
	p          Policy
	hcInterval time.Duration

	from    string
	ignored []string

	tlsConfig     *tls.Config
	tlsServerName string
	maxfails      uint32
	expire        time.Duration
	maxConcurrent int64

	opts options // also here for testing

	// ErrLimitExceeded indicates that a query was rejected because the number of concurrent queries has exceeded
	// the maximum allowed (maxConcurrent)
	ErrLimitExceeded error

	tapPlugin *dnstap.Dnstap // when the dnstap plugin is loaded, we use to this to send messages out.

	Next plugin.Handler
}

// ForwardConfig represents the configuration of the Forward Plugin. This can
// be used with NewWithConfig to create a new configured instance of the
// Forward Plugin.
type ForwardConfig struct {
	From             string
	To               []string
	Except           []string
	MaxFails         *uint32
	HealthCheck      *time.Duration
	HealthCheckNoRec bool
	ForceTCP         bool
	PreferUDP        bool
	TLSConfig        *tls.Config
	TLSServerName    string
	Expire           *time.Duration
	MaxConcurrent    *int64
	Policy           string
	TapPlugin        *dnstap.Dnstap
}

// New returns a new Forward.
func New() *Forward {
	f := &Forward{maxfails: 2, tlsConfig: new(tls.Config), expire: defaultExpire, p: new(random), from: ".", hcInterval: hcInterval, opts: options{forceTCP: false, preferUDP: false, hcRecursionDesired: true}}
	return f
}

// NewWithConfig returns a new Forward configured by the provided
// ForwardConfig.
func NewWithConfig(config ForwardConfig) (*Forward, error) {
	f := New()
	if config.From != "" {
		zones := plugin.Host(config.From).NormalizeExact()
		f.from = zones[0] // there can only be one here, won't work with non-octet reverse

		if len(zones) > 1 {
			log.Warningf("Unsupported CIDR notation: '%s' expands to multiple zones. Using only '%s'.", config.From, f.from)
		}
	}
	for i := 0; i < len(config.Except); i++ {
		f.ignored = append(f.ignored, plugin.Host(config.Except[i]).NormalizeExact()...)
	}
	if config.MaxFails != nil {
		f.maxfails = *config.MaxFails
	}
	if config.HealthCheck != nil {
		if *config.HealthCheck < 0 {
			return nil, fmt.Errorf("health_check can't be negative: %s", *config.HealthCheck)
		}
		f.hcInterval = *config.HealthCheck
	}
	f.opts.hcRecursionDesired = !config.HealthCheckNoRec
	f.opts.forceTCP = config.ForceTCP
	f.opts.preferUDP = config.PreferUDP
	if config.TLSConfig != nil {
		f.tlsConfig = config.TLSConfig
	}
	f.tlsServerName = config.TLSServerName
	if f.tlsServerName != "" {
		f.tlsConfig.ServerName = f.tlsServerName
	}
	if config.Expire != nil {
		f.expire = *config.Expire
		if *config.Expire < 0 {
			return nil, fmt.Errorf("expire can't be negative: %s", *config.Expire)
		}
	}
	if config.MaxConcurrent != nil {
		if *config.MaxConcurrent < 0 {
			return f, fmt.Errorf("max_concurrent can't be negative: %d", *config.MaxConcurrent)
		}
		f.ErrLimitExceeded = fmt.Errorf("concurrent queries exceeded maximum %d", *config.MaxConcurrent)
		f.maxConcurrent = *config.MaxConcurrent
	}
	if config.Policy != "" {
		switch config.Policy {
		case "random":
			f.p = &random{}
		case "round_robin":
			f.p = &roundRobin{}
		case "sequential":
			f.p = &sequential{}
		default:
			return f, fmt.Errorf("unknown policy '%s'", config.Policy)
		}
	}
	f.tapPlugin = config.TapPlugin

	toHosts, err := parse.HostPortOrFile(config.To...)
	if err != nil {
		return f, err
	}

	transports := make([]string, len(toHosts))
	allowedTrans := map[string]bool{"dns": true, "tls": true}
	for i, host := range toHosts {
		trans, h := parse.Transport(host)

		if !allowedTrans[trans] {
			return f, fmt.Errorf("'%s' is not supported as a destination protocol in forward: %s", trans, host)
		}
		p := NewProxy(h, trans)
		f.proxies = append(f.proxies, p)
		transports[i] = trans
	}

	// Initialize ClientSessionCache in tls.Config. This may speed up a TLS handshake
	// in upcoming connections to the same TLS server.
	f.tlsConfig.ClientSessionCache = tls.NewLRUClientSessionCache(len(f.proxies))

	for i := range f.proxies {
		// Only set this for proxies that need it.
		if transports[i] == transport.TLS {
			f.proxies[i].SetTLSConfig(f.tlsConfig)
		}
		f.proxies[i].SetExpire(f.expire)
		f.proxies[i].health.SetRecursionDesired(f.opts.hcRecursionDesired)

	}
	return f, nil
}

// SetProxy appends p to the proxy list and starts healthchecking.
func (f *Forward) SetProxy(p *Proxy) {
	f.proxies = append(f.proxies, p)
	p.start(f.hcInterval)
}

// Len returns the number of configured proxies.
func (f *Forward) Len() int { return len(f.proxies) }

// Name implements plugin.Handler.
func (f *Forward) Name() string { return "forward" }

// ServeDNS implements plugin.Handler.
func (f *Forward) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	state := request.Request{W: w, Req: r}
	if !f.match(state) {
		return plugin.NextOrFailure(f.Name(), f.Next, ctx, w, r)
	}

	if f.maxConcurrent > 0 {
		count := atomic.AddInt64(&(f.concurrent), 1)
		defer atomic.AddInt64(&(f.concurrent), -1)
		if count > f.maxConcurrent {
			MaxConcurrentRejectCount.Add(1)
			return dns.RcodeRefused, f.ErrLimitExceeded
		}
	}

	fails := 0
	var span, child ot.Span
	var upstreamErr error
	span = ot.SpanFromContext(ctx)
	i := 0
	list := f.List()
	deadline := time.Now().Add(defaultTimeout)
	start := time.Now()
	for time.Now().Before(deadline) {
		if i >= len(list) {
			// reached the end of list, reset to begin
			i = 0
			fails = 0
		}

		proxy := list[i]
		i++
		if proxy.Down(f.maxfails) {
			fails++
			if fails < len(f.proxies) {
				continue
			}
			// All upstream proxies are dead, assume healthcheck is completely broken and randomly
			// select an upstream to connect to.
			r := new(random)
			proxy = r.List(f.proxies)[0]

			HealthcheckBrokenCount.Add(1)
		}

		if span != nil {
			child = span.Tracer().StartSpan("connect", ot.ChildOf(span.Context()))
			otext.PeerAddress.Set(child, proxy.addr)
			ctx = ot.ContextWithSpan(ctx, child)
		}

		metadata.SetValueFunc(ctx, "forward/upstream", func() string {
			return proxy.addr
		})

		var (
			ret *dns.Msg
			err error
		)
		opts := f.opts
		for {
			ret, err = proxy.Connect(ctx, state, opts)
			if err == ErrCachedClosed { // Remote side closed conn, can only happen with TCP.
				continue
			}
			// Retry with TCP if truncated and prefer_udp configured.
			if ret != nil && ret.Truncated && !opts.forceTCP && opts.preferUDP {
				opts.forceTCP = true
				continue
			}
			break
		}

		if child != nil {
			child.Finish()
		}

		if f.tapPlugin != nil {
			toDnstap(f, proxy.addr, state, opts, ret, start)
		}

		upstreamErr = err

		if err != nil {
			// Kick off health check to see if *our* upstream is broken.
			if f.maxfails != 0 {
				proxy.Healthcheck()
			}

			if fails < len(f.proxies) {
				continue
			}
			break
		}

		// Check if the reply is correct; if not return FormErr.
		if !state.Match(ret) {
			debug.Hexdumpf(ret, "Wrong reply for id: %d, %s %d", ret.Id, state.QName(), state.QType())

			formerr := new(dns.Msg)
			formerr.SetRcode(state.Req, dns.RcodeFormatError)
			w.WriteMsg(formerr)
			return 0, nil
		}

		w.WriteMsg(ret)
		return 0, nil
	}

	if upstreamErr != nil {
		return dns.RcodeServerFailure, upstreamErr
	}

	return dns.RcodeServerFailure, ErrNoHealthy
}

func (f *Forward) match(state request.Request) bool {
	if !plugin.Name(f.from).Matches(state.Name()) || !f.isAllowedDomain(state.Name()) {
		return false
	}

	return true
}

func (f *Forward) isAllowedDomain(name string) bool {
	if dns.Name(name) == dns.Name(f.from) {
		return true
	}

	for _, ignore := range f.ignored {
		if plugin.Name(ignore).Matches(name) {
			return false
		}
	}
	return true
}

// ForceTCP returns if TCP is forced to be used even when the request comes in over UDP.
func (f *Forward) ForceTCP() bool { return f.opts.forceTCP }

// PreferUDP returns if UDP is preferred to be used even when the request comes in over TCP.
func (f *Forward) PreferUDP() bool { return f.opts.preferUDP }

// List returns a set of proxies to be used for this client depending on the policy in f.
func (f *Forward) List() []*Proxy { return f.p.List(f.proxies) }

var (
	// ErrNoHealthy means no healthy proxies left.
	ErrNoHealthy = errors.New("no healthy proxies")
	// ErrNoForward means no forwarder defined.
	ErrNoForward = errors.New("no forwarder defined")
	// ErrCachedClosed means cached connection was closed by peer.
	ErrCachedClosed = errors.New("cached connection was closed by peer")
)

// options holds various options that can be set.
type options struct {
	forceTCP           bool
	preferUDP          bool
	hcRecursionDesired bool
}

var defaultTimeout = 5 * time.Second
