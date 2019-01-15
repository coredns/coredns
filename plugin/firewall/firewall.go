// Package firewall enables filtering on query and response using direct expression as policy.
// it allows interact with other Policy Engines if those are plugin implementing the Engineer interface
package firewall

import (
	"context"
	"errors"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/plugin/pkg/policy"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var (
	errInvalidAction = errors.New("invalid action")
)

// firewall represents a plugin instance that can validate DNS
// requests and replies using rulelists on the query and/or on the reply
type firewall struct {
	engines map[string]policy.Engine
	query   *ruleList
	reply   *ruleList

	next plugin.Handler
}

func new() (*firewall, error) {
	pol := &firewall{engines: map[string]policy.Engine{"--default--": policy.NewExpressionEngine()}}
	var err error
	if pol.query, err = newRuleList(policy.TypeBlock, false); err != nil {
		return nil, err
	}
	if pol.reply, err = newRuleList(policy.TypeAllow, true); err != nil {
		return nil, err
	}
	return pol, nil
}

// ServeDNS implements the Handler interface.
func (p *firewall) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	var (
		status    = -1
		respMsg   *dns.Msg
		err       error
		queryData = make(map[string]interface{}, 0)
	)

	state := request.Request{W: w, Req: r}

	// ask policy for the Query Rulelist
	action, err := p.query.evaluate(ctx, state, queryData, p.engines)
	if err != nil {
		return p.buildReply(dns.RcodeServerFailure, true, err, w, r)
	}

	if action == policy.TypeAllow {
		// if Allow : ask next plugin to resolve the DNS query
		// temp writer: hold the DNS response until evaluation of the Reply Rulelist
		writer := nonwriter.New(w)
		// RequestDataExtractor requires a Recorder to be able to evaluate the information on the DNS response
		recorder := dnstest.NewRecorder(writer)

		// ask other plugins to resolve
		_, err := plugin.NextOrFailure(p.Name(), p.next, ctx, recorder, r)
		if err != nil {
			return p.buildReply(dns.RcodeServerFailure, true, err, w, r)
		}
		respMsg = writer.Msg

		stateReply := request.Request{W: recorder, Req: respMsg}

		// whatever the response, send to the Reply RuleList for action
		action, err = p.reply.evaluate(ctx, stateReply, queryData, p.engines)
		if err != nil {
			return p.buildReply(dns.RcodeServerFailure, true, err, w, r)
		}
	}

	// Now apply the action evaluated by the RuleLists
	switch action {
	case policy.TypeAllow:
		// the response from resolver, whatever it is, is good to go
		r = respMsg
		status = respMsg.Rcode

	case policy.TypeBlock:
		// One of the RuleList ended evaluation with typeBlock : return the initial request with corresponding rcode
		status = dns.RcodeNameError
	case policy.TypeRefuse:
		// One of the RuleList ended evaluation with typeRefuse : return the initial request with corresponding rcode
		status = dns.RcodeRefused
	case policy.TypeDrop:
		// One of the RuleList ended evaluation with typeDrop : simulate a drop
		return dns.RcodeSuccess, nil
	default:
		// Any other action returned by RuleLists is considered an internal error
		status = dns.RcodeServerFailure
		err = errInvalidAction
	}
	return p.buildReply(status, false, err, w, r)

}

func (p *firewall) buildReply(status int, errorExec bool, err error, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	r.Rcode = status
	r.Response = true
	if status != dns.RcodeServerFailure || errorExec {
		w.WriteMsg(r)
	}

	return dns.RcodeSuccess, err
}

// Name implements the Handler interface
func (p *firewall) Name() string { return "firewall" }
