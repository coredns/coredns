package k8s_node

import (
	"context"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// Nodeer defines the interface that a plugin should implement in order to be used by External.
type Nodeer interface {
	// Node returns a slice of msg.Services that are looked up in the backend and match
	// the request.
	Node(request.Request) ([]msg.Service, int)
	// NodeAddress should return a string slice of addresses for the nameserving endpoint.
	NodeAddress(state request.Request) []dns.RR
}

// Node resolves node hostname IPs from kubernetes clusters.
type Node struct {
	Next  plugin.Handler
	Zones []string

	hostmaster string
	ttl        uint32

	nodeFunc     func(request.Request) ([]msg.Service, int)
	nodeAddrFunc func(request.Request) []dns.RR
}

// New returns a new and initialized *Node.
func New() *Node {
	e := &Node{hostmaster: "node"}
	return e
}

// ServeDNS implements the plugin.Handle interface.
func (e *Node) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	node, rcode := e.nodeFunc(state)

	if rcode != dns.RcodeSuccess {
		return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
	}
	answers := []dns.RR{}

	switch state.QType() {
	case dns.TypeA:
		answers = e.a(node, state)
	case dns.TypeAAAA:
		answers = e.aaaa(node, state)
	}

	// If we did have records, but queried for the wrong qtype return a nodata response.
	if len(answers) == 0 {
		return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
	}

	m := new(dns.Msg)
	m.SetReply(state.Req)
	m.Authoritative = true
	m.Answer = answers

	w.WriteMsg(m)
	return rcode, nil
}

// Name implements the Handler interface.
func (e *Node) Name() string { return "k8s_node" }
