// Package dnsserver implements all the interfaces from Caddy, so that CoreDNS can be a servertype plugin.
package dnsserver

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics/vars"
	"github.com/coredns/coredns/plugin/pkg/dso"
	"github.com/coredns/coredns/plugin/pkg/edns"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/rcode"
	"github.com/coredns/coredns/plugin/pkg/reuseport"
	"github.com/coredns/coredns/plugin/pkg/trace"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	ot "github.com/opentracing/opentracing-go"
)

// Server represents an instance of a server, which serves
// DNS requests at a particular address (host and port). A
// server is capable of serving numerous zones on
// the same address and the listener may be stopped for
// graceful termination (POSIX only).
type Server struct {
	Addr string // Address we listen on

	server [2]*dns.Server // 0 is a net.Listener, 1 is a net.PacketConn (a *UDPConn) in our case.
	m      sync.Mutex     // protects the servers

	zones        map[string][]*Config // zones keyed by their address
	dnsWg        sync.WaitGroup       // used to wait on outstanding connections
	graceTimeout time.Duration        // the maximum duration of a graceful shutdown
	trace        trace.Trace          // the trace plugin for the server
	debug        bool                 // disable recover()
	stacktrace   bool                 // enable stacktrace in recover error log
	classChaos   bool                 // allow non-INET class queries
	idleTimeout  time.Duration        // Idle timeout for TCP
	readTimeout  time.Duration        // Read timeout for TCP
	writeTimeout time.Duration        // Write timeout for TCP

	tsigSecret map[string]string

	opcodeDSO    bool                      // allow RFC 8490 DNS Stateful Operations messages
	dsoSessions  map[net.Conn]*dso.Session // DSO sessions of active TCP connections
	dsoSessionsM sync.RWMutex              // protects the DSO sessions
}

// MetadataCollector is a plugin that can retrieve metadata functions from all metadata providing plugins
type MetadataCollector interface {
	Collect(context.Context, request.Request) context.Context
}

// NewServer returns a new CoreDNS server and compiles all plugins in to it. By default CH class
// queries are blocked unless queries from enableChaos are loaded.
func NewServer(addr string, group []*Config) (*Server, error) {
	s := &Server{
		Addr:         addr,
		zones:        make(map[string][]*Config),
		graceTimeout: 5 * time.Second,
		idleTimeout:  10 * time.Second,
		readTimeout:  3 * time.Second,
		writeTimeout: 5 * time.Second,
		tsigSecret:   make(map[string]string),
	}

	// We have to bound our wg with one increment
	// to prevent a "race condition" that is hard-coded
	// into sync.WaitGroup.Wait() - basically, an add
	// with a positive delta must be guaranteed to
	// occur before Wait() is called on the wg.
	// In a way, this kind of acts as a safety barrier.
	s.dnsWg.Add(1)

	for _, site := range group {
		if site.Debug {
			s.debug = true
			log.D.Set()
		}
		s.stacktrace = site.Stacktrace

		// append the config to the zone's configs
		s.zones[site.Zone] = append(s.zones[site.Zone], site)

		// set timeouts
		if site.ReadTimeout != 0 {
			s.readTimeout = site.ReadTimeout
		}
		if site.WriteTimeout != 0 {
			s.writeTimeout = site.WriteTimeout
		}
		if site.IdleTimeout != 0 {
			s.idleTimeout = site.IdleTimeout
		}

		// copy tsig secrets
		for key, secret := range site.TsigSecret {
			s.tsigSecret[key] = secret
		}

		// compile custom plugin for everything
		var stack plugin.Handler
		var dsoStack plugin.DSOHandler
		for i := len(site.Plugin) - 1; i >= 0; i-- {
			stack = site.Plugin[i](stack)

			if h, ok := stack.(plugin.DSOHandler); ok {
				h.SetNextDSO(dsoStack)
				dsoStack = h
				s.opcodeDSO = true
			}

			// register the *handler* also
			site.registerHandler(stack)

			// If the current plugin is a MetadataCollector, bookmark it for later use. This loop traverses the plugin
			// list backwards, so the first MetadataCollector plugin wins.
			if mdc, ok := stack.(MetadataCollector); ok {
				site.metaCollector = mdc
			}

			if s.trace == nil && stack.Name() == "trace" {
				// we have to stash away the plugin, not the
				// Tracer object, because the Tracer won't be initialized yet
				if t, ok := stack.(trace.Trace); ok {
					s.trace = t
				}
			}
			// Unblock CH class queries when any of these plugins are loaded.
			if _, ok := EnableChaos[stack.Name()]; ok {
				s.classChaos = true
			}
		}
		site.pluginChain = stack
		site.dsoPluginChain = dsoStack
	}

	if !s.debug {
		// When reloading we need to explicitly disable debug logging if it is now disabled.
		log.D.Clear()
	}

	return s, nil
}

// Compile-time check to ensure Server implements the caddy.GracefulServer interface
var _ caddy.GracefulServer = &Server{}

// Serve starts the server with an existing listener. It blocks until the server stops.
// This implements caddy.TCPServer interface.
func (s *Server) Serve(l net.Listener) error {
	s.m.Lock()

	s.server[tcp] = &dns.Server{Listener: l,
		Net:           "tcp",
		TsigSecret:    s.tsigSecret,
		MaxTCPQueries: tcpMaxQueries,
		ReadTimeout:   s.readTimeout,
		WriteTimeout:  s.writeTimeout,
		IdleTimeout: func() time.Duration {
			return s.idleTimeout
		},
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
			ctx := context.WithValue(context.Background(), Key{}, s)
			ctx = context.WithValue(ctx, LoopKey{}, 0)
			s.ServeDNS(ctx, w, r)
		})}
	if s.opcodeDSO {
		s.dsoSessions = make(map[net.Conn]*dso.Session)
		s.server[tcp].MsgAcceptFunc = func(dh dns.Header) dns.MsgAcceptAction {
			opcode := int(dh.Bits>>11) & 0xF
			if opcode == dns.OpcodeStateful {
				// RFC 8490, Section 5.4: If ... any of the count fields are not zero,
				// then a FORMERR MUST be returned.
				if dh.Qdcount != 0 || dh.Ancount != 0 || dh.Nscount != 0 || dh.Arcount != 0 {
					return dns.MsgReject
				}
				return dns.MsgAccept
			}
			return dns.DefaultMsgAcceptFunc(dh)
		}
	}

	s.m.Unlock()

	return s.server[tcp].ActivateAndServe()
}

// ServePacket starts the server with an existing packetconn. It blocks until the server stops.
// This implements caddy.UDPServer interface.
func (s *Server) ServePacket(p net.PacketConn) error {
	s.m.Lock()
	s.server[udp] = &dns.Server{PacketConn: p, Net: "udp", Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		ctx := context.WithValue(context.Background(), Key{}, s)
		ctx = context.WithValue(ctx, LoopKey{}, 0)
		s.ServeDNS(ctx, w, r)
	}), TsigSecret: s.tsigSecret}
	s.m.Unlock()

	return s.server[udp].ActivateAndServe()
}

// Listen implements caddy.TCPServer interface.
func (s *Server) Listen() (net.Listener, error) {
	l, err := reuseport.Listen("tcp", s.Addr[len(transport.DNS+"://"):])
	if err != nil {
		return nil, err
	}
	return l, nil
}

// WrapListener Listen implements caddy.GracefulServer interface.
func (s *Server) WrapListener(ln net.Listener) net.Listener {
	return ln
}

// ListenPacket implements caddy.UDPServer interface.
func (s *Server) ListenPacket() (net.PacketConn, error) {
	p, err := reuseport.ListenPacket("udp", s.Addr[len(transport.DNS+"://"):])
	if err != nil {
		return nil, err
	}

	return p, nil
}

// Stop stops the server. It blocks until the server is
// totally stopped. On POSIX systems, it will wait for
// connections to close (up to a max timeout of a few
// seconds); on Windows it will close the listener
// immediately.
// This implements Caddy.Stopper interface.
func (s *Server) Stop() (err error) {
	if runtime.GOOS != "windows" {
		// force connections to close after timeout
		done := make(chan struct{})
		go func() {
			s.dnsWg.Done() // decrement our initial increment used as a barrier
			s.dnsWg.Wait()
			close(done)
		}()

		// Wait for remaining connections to finish or
		// force them all to close after timeout
		select {
		case <-time.After(s.graceTimeout):
		case <-done:
		}
	}

	// Close the listener now; this stops the server without delay
	s.m.Lock()
	for _, s1 := range s.server {
		// We might not have started and initialized the full set of servers
		if s1 != nil {
			err = s1.Shutdown()
		}
	}
	s.m.Unlock()
	return
}

// Address together with Stop() implement caddy.GracefulServer.
func (s *Server) Address() string { return s.Addr }

// ServeDNS is the entry point for every request to the address that
// is bound to. It acts as a multiplexer for the requests zonename as
// defined in the request so that the correct zone
// (configuration and plugin stack) will handle the request.
func (s *Server) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
	// The default dns.Mux checks the question section size, but we have our
	// own mux here. Check if we have a question section. If not drop them here.
	if r == nil {
		errorAndMetricsFunc(s.Addr, w, r, dns.RcodeServerFailure)
		return
	}
	req := request.Request{Req: r, W: w}
	if r.Opcode == dns.OpcodeStateful && req.Proto() != "tcp" {
		errorAndMetricsFunc(s.Addr, w, r, dns.RcodeServerFailure)
		return
	}
	if r.Opcode == dns.OpcodeStateful && !s.opcodeDSO {
		errorAndMetricsFunc(s.Addr, w, r, dns.RcodeNotImplemented)
		return
	}
	if r.Opcode != dns.OpcodeStateful && len(r.Question) == 0 {
		errorAndMetricsFunc(s.Addr, w, r, dns.RcodeServerFailure)
		return
	}

	if !s.debug {
		defer func() {
			// In case the user doesn't enable error plugin, we still
			// need to make sure that we stay alive up here
			if rec := recover(); rec != nil {
				if s.stacktrace {
					log.Errorf("Recovered from panic in server: %q %v\n%s", s.Addr, rec, string(debug.Stack()))
				} else {
					log.Errorf("Recovered from panic in server: %q %v", s.Addr, rec)
				}
				vars.Panic.Inc()
				errorAndMetricsFunc(s.Addr, w, r, dns.RcodeServerFailure)
			}
		}()
	}

	if !s.classChaos && r.Opcode != dns.OpcodeStateful && r.Question[0].Qclass != dns.ClassINET {
		errorAndMetricsFunc(s.Addr, w, r, dns.RcodeRefused)
		return
	}

	if m, err := edns.Version(r); err != nil { // Wrong EDNS version, return at once.
		w.WriteMsg(m)
		return
	}

	// Wrap the response writer in a ScrubWriter so we automatically make the reply fit in the client's buffer.
	w = request.NewScrubWriter(r, w)

	req = request.Request{Req: r, W: w}
	q := req.Name()
	var (
		off       int
		end       bool
		dshandler *Config
	)

	for {
		if z, ok := s.zones[q[off:]]; ok {
			for _, h := range z {
				if h.pluginChain == nil { // zone defined, but has not got any plugins
					errorAndMetricsFunc(s.Addr, w, r, dns.RcodeRefused)
					return
				}
				if r.Opcode == dns.OpcodeStateful && h.dsoPluginChain == nil {
					errorAndMetricsFunc(s.Addr, w, r, dns.RcodeNotImplemented)
					return
				}

				if h.metaCollector != nil {
					// Collect metadata now, so it can be used before we send a request down the plugin chain.
					ctx = h.metaCollector.Collect(ctx, req)
				}

				// If all filter funcs pass, use this config.
				if passAllFilterFuncs(ctx, h.FilterFuncs, &req) {
					if r.Opcode != dns.OpcodeStateful && r.Question[0].Qtype == dns.TypeDS {
						// The type is DS, keep the handler, but keep on searching as maybe we are serving
						// the parent as well and the DS should be routed to it - this will probably *misroute* DS
						// queries to a possibly grand parent, but there is no way for us to know at this point
						// if there is an actual delegation from grandparent -> parent -> zone.
						// In all fairness: direct DS queries should not be needed.
						dshandler = h
						continue
					}

					if h.ViewName != "" {
						// if there was a view defined for this Config, set the view name in the context
						ctx = context.WithValue(ctx, ViewKey{}, h.ViewName)
					}
					var rcode int
					switch r.Opcode {
					case dns.OpcodeStateful:
						rcode, _ = h.dsoPluginChain.ServeDSO(ctx, w, r)
					default:
						rcode, _ = h.pluginChain.ServeDNS(ctx, w, r)
					}
					if !plugin.ClientWrite(rcode) {
						errorFunc(s.Addr, w, r, rcode)
					}
					return
				}
			}
		}
		off, end = dns.NextLabel(q, off)
		if end {
			break
		}
	}

	if dshandler != nil {
		// DS request, and we found a zone, use the handler for the query.
		rcode, _ := dshandler.pluginChain.ServeDNS(ctx, w, r)
		if !plugin.ClientWrite(rcode) {
			errorFunc(s.Addr, w, r, rcode)
		}
		return
	}

	// Wildcard match, if we have found nothing try the root zone as a last resort.
	if z, ok := s.zones["."]; ok {
		for _, h := range z {
			if h.pluginChain == nil {
				continue
			}
			if r.Opcode == dns.OpcodeStateful && h.dsoPluginChain == nil {
				continue
			}

			if h.metaCollector != nil {
				// Collect metadata now, so it can be used before we send a request down the plugin chain.
				ctx = h.metaCollector.Collect(ctx, req)
			}

			// If all filter funcs pass, use this config.
			if passAllFilterFuncs(ctx, h.FilterFuncs, &req) {
				if h.ViewName != "" {
					// if there was a view defined for this Config, set the view name in the context
					ctx = context.WithValue(ctx, ViewKey{}, h.ViewName)
				}
				var rcode int
				switch {
				case r.Opcode == dns.OpcodeStateful:
					rcode, _ = h.dsoPluginChain.ServeDSO(ctx, w, r)
				default:
					rcode, _ = h.pluginChain.ServeDNS(ctx, w, r)
				}
				if !plugin.ClientWrite(rcode) {
					errorFunc(s.Addr, w, r, rcode)
				}
				return
			}
		}
	}

	// Still here? Error out with REFUSED.
	errorAndMetricsFunc(s.Addr, w, r, dns.RcodeRefused)
}

// passAllFilterFuncs returns true if all filter funcs evaluate to true for the given request
func passAllFilterFuncs(ctx context.Context, filterFuncs []FilterFunc, req *request.Request) bool {
	for _, ff := range filterFuncs {
		if !ff(ctx, req) {
			return false
		}
	}
	return true
}

// OnStartupComplete lists the sites served by this server
// and any relevant information, assuming Quiet is false.
func (s *Server) OnStartupComplete() {
	if Quiet {
		return
	}

	out := startUpZones("", s.Addr, s.zones)
	if out != "" {
		fmt.Print(out)
	}
}

// Tracer returns the tracer in the server if defined.
func (s *Server) Tracer() ot.Tracer {
	if s.trace == nil {
		return nil
	}

	return s.trace.Tracer()
}

// DSOSession returns appropriate DSO session for a given message.
//
// Returns nil if none of server's configs has a plugin that implements DSOHandler
// or when called from an inappropriate context such as UDP.
func (s *Server) DSOSession(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) *dso.Session {
	if !s.opcodeDSO {
		return nil
	}
	state := request.Request{Req: r, W: w}
	if state.Proto() != "tcp" {
		return nil
	}

	we := w.(dns.ResponseWriterExtra)
	conn := we.Conn()
	connCtx := we.Context()

	s.dsoSessionsM.Lock()
	defer s.dsoSessionsM.Unlock()

	sesh, ok := s.dsoSessions[conn]
	if !ok {
		// If context is already cancelled (thus connection is closed) the AfterFunc
		// below would be called immediatelly. Avoid the roundtrip.
		if connCtx.Err() != nil {
			return dso.NewClosedSession(s.idleTimeout)
		}

		sesh = dso.NewSession(s.idleTimeout)
		s.dsoSessions[conn] = sesh
		context.AfterFunc(connCtx, func() {
			s.dsoSessionsM.Lock()
			delete(s.dsoSessions, conn)
			s.dsoSessionsM.Unlock()
		})
	}
	return sesh
}

// errorFunc responds to an DNS request with an error.
func errorFunc(server string, w dns.ResponseWriter, r *dns.Msg, rc int) {
	state := request.Request{W: w, Req: r}

	answer := new(dns.Msg)
	answer.SetRcode(r, rc)
	state.SizeAndDo(answer)

	w.WriteMsg(answer)
}

func errorAndMetricsFunc(server string, w dns.ResponseWriter, r *dns.Msg, rc int) {
	state := request.Request{W: w, Req: r}

	answer := new(dns.Msg)
	answer.SetRcode(r, rc)
	state.SizeAndDo(answer)

	vars.Report(server, state, vars.Dropped, "", rcode.ToString(rc), "" /* plugin */, answer.Len(), time.Now())

	w.WriteMsg(answer)
}

const (
	tcp = 0
	udp = 1

	tcpMaxQueries = -1
)

type (
	// Key is the context key for the current server added to the context.
	Key struct{}

	// LoopKey is the context key to detect server wide loops.
	LoopKey struct{}

	// ViewKey is the context key for the current view, if defined
	ViewKey struct{}
)

// EnableChaos is a map with plugin names for which we should open CH class queries as we block these by default.
var EnableChaos = map[string]struct{}{
	"chaos":   {},
	"forward": {},
	"proxy":   {},
}

// Quiet mode will not show any informative output on initialization.
var Quiet bool
