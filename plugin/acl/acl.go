package acl

import (
	"context"
	"net"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/infobloxopen/go-trees/iptree"
	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("acl")

type acl struct {
	Next plugin.Handler

	Rules []Rule
}

// Rule defines a list of Zones and some ACL policies which will be
// enforced on them.
type Rule struct {
	Zones    []string
	Policies []Policy
}

// Policy defines the ACL policy for DNS queries.
// A policy performs the specified action (block/allow) on all DNS queries
// matched by source IP or QTYPE.
type Policy struct {
	action int
	qtypes map[uint16]struct{}
	filter *iptree.Tree
}

const (
	// ALLOW allows authorized queries to recurse.
	ALLOW = iota
	// BLOCK blocks unauthorized queries towards protected DNS zones.
	BLOCK
)

func (a acl) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	for _, rule := range a.Rules {
		// check zone.
		zone := plugin.Zones(rule.Zones).Matches(state.Name())
		if zone == "" {
			continue
		}

		if shouldBlock(rule.Policies, w, r) {
			m := new(dns.Msg)
			m.SetRcode(r, dns.RcodeRefused)
			w.WriteMsg(m)
			RequestBlockCount.WithLabelValues(metrics.WithServer(ctx), zone).Inc()
			// TODO: should we return Success here? (@ihac)
			return dns.RcodeSuccess, nil
		}
	}
	RequestAllowCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
	return plugin.NextOrFailure(state.Name(), a.Next, ctx, w, r)
}

func shouldBlock(policies []Policy, w dns.ResponseWriter, r *dns.Msg) bool {
	state := request.Request{W: w, Req: r}

	ip := net.ParseIP(state.IP())
	qtype := state.QType()
	for _, policy := range policies {
		_, matchAll := policy.qtypes[QtypeAll]
		_, match := policy.qtypes[qtype]
		if !matchAll && !match {
			continue
		}

		_, contained := policy.filter.GetByIP(ip)
		if !contained {
			continue
		}

		// matched.
		switch policy.action {
		case ALLOW:
			return false
		case BLOCK:
			return true
		}
	}
	return false
}

func (a acl) Name() string {
	return "acl"
}
