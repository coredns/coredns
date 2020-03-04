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
	"time"
)

var log = clog.NewWithPlugin("fanout")

// Fanout represents a plugin instance that can do async requests to list of DNS servers.
type Fanout struct {
	clients       []Client
	tlsConfig     *tls.Config
	ignored       []string
	tlsServerName string
	net           string
	from          string
	workerCount   int
	Next          plugin.Handler
}

// New returns reference to new Fanout plugin instance with default configs.
func New() *Fanout {
	return &Fanout{
		tlsConfig: new(tls.Config),
		net:       "udp",
	}
}

func (f *Fanout) addClient(p Client) {
	f.clients = append(f.clients, p)
	f.workerCount++
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
	responseCh := make(chan *response, clientCount)
	go func() {
		for i := 0; i < clientCount; i++ {
			client := f.clients[i]
			workerChannel <- client
		}
	}()
	for i := 0; i < f.workerCount; i++ {
		go func() {
			for c := range workerChannel {
				start := time.Now()
				msg, err := c.Request(request.Request{W: w, Req: m})
				responseCh <- &response{client: c, response: msg, start: start, err: err}
			}
		}()
	}
	result := f.getFanoutResult(timeoutContext, responseCh)
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
		logErrIfNotNil(w.WriteMsg(formerr))
		return 0, dnsTAP
	}
	logErrIfNotNil(w.WriteMsg(result.response))
	return 0, dnsTAP
}

func (f *Fanout) getFanoutResult(ctx context.Context, responseCh <-chan *response) *response {
	count := len(f.clients)
	var result *response
	for {
		select {
		case <-ctx.Done():
			return result
		case r := <-responseCh:
			count--
			if isBetter(result, r) {
				result = r
			}
			if count == 0 {
				return result
			}
			if r.err != nil {
				break
			}
			if r.response.Rcode != dns.RcodeSuccess {
				break
			}
			return r
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
