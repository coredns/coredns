package fanout

import (
	"context"
	"crypto/tls"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/debug"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
)

var log = clog.NewWithPlugin("fanout")

// Fanout represents a plugin instance that can do async requests to list of DNS servers.
type Fanout struct {
	clients       []Client
	policy        Policy
	tlsConfig     *tls.Config
	ignored       []string
	tlsServerName string
	net           string
	from          string
	workerCount   int
	maxFailCount  int
	Next          plugin.Handler
}

// New returns reference to new Fanout plugin instance with default configs.
func New() *Fanout {
	return &Fanout{
		tlsConfig:    new(tls.Config),
		maxFailCount: 2,
		workerCount:  4,
		net:          "udp",
		policy:       FirstPositive,
	}
}

func (f *Fanout) addClient(p Client) {
	f.clients = append(f.clients, p)
}

// Name implements plugin.Handler.
func (f *Fanout) Name() string {
	return "fanout"
}

// ServeDNS implements plugin.Handler.
func (f *Fanout) ServeDNS(ctx context.Context, w dns.ResponseWriter, m *dns.Msg) (int, error) {
	req := request.Request{W: w, Req: m}
	if !f.match(req) {
		return plugin.NextOrFailure(f.Name(), f.Next, ctx, w, m)
	}
	timeoutContext, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	clientCount := len(f.clients)
	workerChannel := make(chan Client, f.workerCount)
	resultCh := make(chan connectResult, clientCount)
	go func() {
		for i := 0; i < clientCount; i++ {
			client := f.clients[i]
			workerChannel <- client
		}
	}()
	for i := 0; i < f.workerCount; i++ {
		go func() {
			for c := range workerChannel {
				connect(timeoutContext, c, request.Request{W: w, Req: m}, resultCh, f.maxFailCount)
			}
		}()
	}
	result := f.getFanoutResult(timeoutContext, resultCh)
	if result == nil {
		return dns.RcodeServerFailure, timeoutContext.Err()
	}
	if result.err != nil {
		return dns.RcodeServerFailure, result.err
	}
	dnsTAP := toDnstap(ctx, result.client.Endpoint(), f.net, req, result.response, result.start)
	if !req.Match(result.response) {
		debug.Hexdumpf(result.response, "Wrong reply for id: %d, %s %d", result.response.Id, req.QName(), req.QType())
		formerr := new(dns.Msg)
		formerr.SetRcode(req.Req, dns.RcodeFormatError)
		logIfNotNil(w.WriteMsg(formerr))
		return 0, dnsTAP
	}
	logIfNotNil(w.WriteMsg(result.response))
	return 0, dnsTAP
}

func (f *Fanout) getFanoutResult(ctx context.Context, ch <-chan connectResult) *connectResult {
	count := len(f.clients)
	var result *connectResult
	for {
		select {
		case <-ctx.Done():
			return result
		case r := <-ch:
			count--
			if isBetter(result, &r) {
				result = &r
			}
			if count == 0 {
				return result
			}
			if r.err != nil {
				break
			}
			if !f.checkResponse(r.response) {
				break
			}
			return &r
		}
	}
}

func (f *Fanout) match(state request.Request) bool {
	if !plugin.Name(f.from).Matches(state.Name()) || !f.isAllowedDomain(state.Name()) {
		return false
	}
	return true
}

func (f *Fanout) isAllowedDomain(name string) bool {
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

func (f *Fanout) checkResponse(r *dns.Msg) bool {
	switch f.policy {
	case FirstPositive:
		return r.Rcode == dns.RcodeSuccess
	case Any:
		return true
	}
	return false
}
