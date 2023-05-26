package acl

import (
	"context"
	"net"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/infobloxopen/go-trees/iptree"
	"github.com/miekg/dns"
)

// ACL enforces access control policies on DNS queries.
type ACL struct {
	Next plugin.Handler

	Rules []rule
}

// rule defines a list of Zones and some ACL policies which will be
// enforced on them.
type rule struct {
	zones    []string
	policies []policy
}

// action defines the action against queries.
type action int

// policy defines the ACL policy for DNS queries.
// A policy performs the specified action (block/allow) on all DNS queries
// matched by source IP or QTYPE.
type policy struct {
	action action
	qtypes map[uint16]struct{}
	filter *iptree.Tree
}

const (
	// actionNone does nothing on the queries.
	actionNone = iota
	// actionAllow allows authorized queries to recurse.
	actionAllow
	// actionBlock blocks unauthorized queries towards protected DNS zones.
	actionBlock
	// actionFilter returns empty sets for queries towards protected DNS zones.
	actionFilter
	// actionDrop does not respond for queries towards the protected DNS zones.
	actionDrop
)

var log = clog.NewWithPlugin("acl")

// ServeDNS implements the plugin.Handler interface.
func (a ACL) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

RulesCheckLoop:
	for _, rule := range a.Rules {
		// check zone.
		zone := plugin.Zones(rule.zones).Matches(state.Name())
		if zone == "" {
			continue
		}

		action := matchWithPolicies(rule.policies, w, r)
		switch action {
		case actionDrop:
			{
				RequestDropCount.WithLabelValues(metrics.WithServer(ctx), zone, metrics.WithView(ctx)).Inc()
				return dns.RcodeSuccess, nil
			}
		case actionBlock:
			{
				m := new(dns.Msg).
					SetRcode(r, dns.RcodeRefused).
					SetEdns0(4096, true)
				ede := dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeBlocked}
				m.IsEdns0().Option = append(m.IsEdns0().Option, &ede)
				w.WriteMsg(m)
				RequestBlockCount.WithLabelValues(metrics.WithServer(ctx), zone, metrics.WithView(ctx)).Inc()
				return dns.RcodeSuccess, nil
			}
		case actionAllow:
			{
				break RulesCheckLoop
			}
		case actionFilter:
			{
				m := new(dns.Msg).
					SetRcode(r, dns.RcodeSuccess).
					SetEdns0(4096, true)
				ede := dns.EDNS0_EDE{InfoCode: dns.ExtendedErrorCodeFiltered}
				m.IsEdns0().Option = append(m.IsEdns0().Option, &ede)
				w.WriteMsg(m)
				RequestFilterCount.WithLabelValues(metrics.WithServer(ctx), zone, metrics.WithView(ctx)).Inc()
				return dns.RcodeSuccess, nil
			}
		}
	}

	RequestAllowCount.WithLabelValues(metrics.WithServer(ctx), metrics.WithView(ctx)).Inc()
	return plugin.NextOrFailure(state.Name(), a.Next, ctx, w, r)
}

// matchWithPolicies matches the DNS query with a list of ACL polices and returns suitable
// action against the query.
func matchWithPolicies(policies []policy, w dns.ResponseWriter, r *dns.Msg) action {
	state := request.Request{W: w, Req: r}

	var ip net.IP
	if idx := strings.IndexByte(state.IP(), '%'); idx >= 0 {
		ip = net.ParseIP(state.IP()[:idx])
	} else {
		ip = net.ParseIP(state.IP())
	}

	// if the parsing did not return a proper response then we simply return 'actionBlock' to
	// block the query
	if ip == nil {
		log.Errorf("Blocking request. Unable to parse source address: %v", state.IP())
		return actionBlock
	}
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
	return actionNone
}

// Name implements the plugin.Handler interface.
func (a ACL) Name() string {
	return "acl"
}
