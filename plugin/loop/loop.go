package loop

import (
	"context"
	"sync"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("loop")

// Loop is a plugin that implements loop detection by sending a "random" query.
type Loop struct {
	Next plugin.Handler

	zone  string
	qname string

	sync.RWMutex
	i   int
	off bool
}

// New returns a new initialized Loop.
func New(zone string) *Loop { return &Loop{zone: zone, qname: qname(zone)} }

// ServeDNS implements the plugin.Handler interface.
func (l *Loop) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if r.Question[0].Qtype != dns.TypeHINFO {
		return plugin.NextOrFailure(l.Name(), l.Next, ctx, w, r)
	}
	if l.disabled() {
		return plugin.NextOrFailure(l.Name(), l.Next, ctx, w, r)
	}

	state := request.Request{W: w, Req: r}

	zone := plugin.Zones([]string{l.zone}).Matches(state.Name())
	if zone == "" {
		return plugin.NextOrFailure(l.Name(), l.Next, ctx, w, r)
	}

	if state.Name() == l.qname {
		l.inc()
	}

	if l.seen() > 2 {
		zone := dnsutil.Join(dns.SplitDomainName(l.qname)[2:]...) + "."
		log.Fatalf("Forwarding loop detected in \"%s\" zone. Probe \"HINFO IN %s\" was seen more than twice.", zone, l.qname)
	}

	return plugin.NextOrFailure(l.Name(), l.Next, ctx, w, r)
}

// Name implements the plugin.Handler interface.
func (l *Loop) Name() string { return "loop" }

func (l *Loop) exchange(addr string) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(l.qname, dns.TypeHINFO)

	return dns.Exchange(m, addr)
}

func (l *Loop) seen() int {
	l.RLock()
	defer l.RUnlock()
	return l.i
}

func (l *Loop) inc() {
	l.Lock()
	defer l.Unlock()
	l.i++
}

func (l *Loop) setDisabled() {
	l.Lock()
	defer l.Unlock()
	l.off = true
}

func (l *Loop) disabled() bool {
	l.RLock()
	defer l.RUnlock()
	return l.off
}
