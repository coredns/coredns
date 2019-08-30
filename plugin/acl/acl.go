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

// Action defines the action against queries.
type Action int

// Policy defines the ACL policy for DNS queries.
// A policy performs the specified action (block/allow) on all DNS queries
// matched by source IP or QTYPE.
type Policy struct {
	action Action
	qtypes map[uint16]struct{}
	filter *iptree.Tree
}

const (
	// ActionNone does nothing on the queries.
	ActionNone = iota
	// ActionAllow allows authorized queries to recurse.
	ActionAllow
	// ActionBlock blocks unauthorized queries towards protected DNS zones.
	ActionBlock
)

func (a acl) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

RulesCheckLoop:
	for _, rule := range a.Rules {
		// check zone.
		zone := plugin.Zones(rule.Zones).Matches(state.Name())
		if zone == "" {
			continue
		}

		action := matchWithPolicies(rule.Policies, w, r)
		switch action {
		case ActionBlock:
			{
				m := new(dns.Msg)
				m.SetRcode(r, dns.RcodeRefused)
				w.WriteMsg(m)
				RequestBlockCount.WithLabelValues(metrics.WithServer(ctx), zone).Inc()
				return dns.RcodeSuccess, nil
			}
		case ActionAllow:
			{
				break RulesCheckLoop
			}
		}
	}

	RequestAllowCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
	return plugin.NextOrFailure(state.Name(), a.Next, ctx, w, r)
}

// matchWithPolicies matches the DNS query with a list of ACL polices and returns suitable
// action agains the query.
func matchWithPolicies(policies []Policy, w dns.ResponseWriter, r *dns.Msg) Action {
	state := request.Request{W: w, Req: r}

	ip := net.ParseIP(state.IP())
	qtype := state.QType()
	for _, policy := range policies {
		// dns.TypeNone matches all query types.
		_, matchAll := policy.qtypes[dns.TypeNone]
		_, match := policy.qtypes[qtype]
		if !matchAll && !match {
			continue
		}

		_, contained := policy.filter.GetByIP(ip)
		if !contained {
			continue
		}

		// matched.
		return policy.action
	}
	return ActionNone
}

func (a acl) Name() string {
	return "acl"
}
