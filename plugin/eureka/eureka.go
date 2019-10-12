// Package eureka implements a plugin that returns resource records
// from Netflix Eureka.
package eureka

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

const (
	modeApp mode = "app"
	modeVip mode = "vip"
)

// Eureka is a plugin that returns resource records from Netflix Eureka.
type Eureka struct {
	Next  plugin.Handler
	Fall  fall.F
	Zones []string

	options *options
	client  clientAPI

	zMu       sync.RWMutex
	instances *instancesMap
}

type instancesMap map[string][]*instance

type mode string

type options struct {
	refresh time.Duration
	ttl     uint32
	mode    mode
}

// New returns a new and initialized *Eureka.
func New(ctx context.Context, options *options, client clientAPI) (*Eureka, error) {
	return &Eureka{
		client:    client,
		options:   options,
		instances: &instancesMap{},
	}, nil
}

// Run executes first update, spins up an update forever-loop.
// Returns error if first update fails.
func (e *Eureka) Run(ctx context.Context) error {
	if err := e.updateApps(ctx); err != nil {
		return err
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Infof("Breaking out of Eureka update loop: %v", ctx.Err())
				return
			case <-time.After(e.options.refresh):
				if err := e.updateApps(ctx); err != nil && ctx.Err() == nil /* Don't log error if ctx expired. */ {
					log.Errorf("Failed to update Eureka apps: %v", err)
				}
			}
		}
	}()
	return nil
}

// ServeDNS implements the plugin.Handler.ServeDNS.
func (e *Eureka) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	if state.QType() != dns.TypeA {
		// Eureka only supports IPv4
		return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
	}

	qname := state.Name()

	zName := plugin.Zones(e.Zones).Matches(qname)
	if zName == "" {
		return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	key := strings.TrimSuffix(qname, "."+zName)

	e.zMu.RLock()
	instances, ok := (*e.instances)[key]
	if ok {
		m.Answer = e.a(instances, state)
	}
	e.zMu.RUnlock()

	if (m.Answer == nil || len(m.Answer) == 0) && e.Fall.Through(qname) {
		return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
	}

	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func (e *Eureka) updateApps(ctx context.Context) error {
	apps, err := e.client.fetchAllApplications()
	if err != nil {
		return fmt.Errorf("errors fetching eureka applications: %v", err)
	}

	newInstances := instancesMap{}
	for _, app := range apps.Application {
		for _, instance := range app.Instance {
			if instance.Status == statusUp {
				var key string
				if e.options.mode == modeApp {
					key = app.Name
				} else if e.options.mode == modeVip {
					key = instance.VipAddress
				}
				key = strings.ToLower(key)
				newInstances[key] = append(newInstances[key], instance)
			}
		}
	}
	e.zMu.Lock()
	e.instances = &newInstances
	e.zMu.Unlock()
	return nil
}

func (e *Eureka) a(instances []*instance, state request.Request) (records []dns.RR) {
	for _, a := range instances {
		record := &dns.A{
			Hdr: dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: e.options.ttl},
			A:   net.ParseIP(a.IpAddr),
		}
		records = append(records, record)
	}
	return records
}

// Name implements plugin.Handler.Name.
func (e *Eureka) Name() string { return "eureka" }
