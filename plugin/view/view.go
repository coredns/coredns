package view

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/expression"
	"github.com/coredns/coredns/request"

	"github.com/antonmedv/expr"
	"github.com/antonmedv/expr/vm"
	"github.com/miekg/dns"
)

// View is a plugin that enables configuring expression based advanced routing
type View struct {
	progs      []*vm.Program
	Next       plugin.Handler
}

// Filter implements dnsserver.Viewer.  It returns true if all View rules evaluate to true for the given state.
func (v *View) Filter(state *request.Request) bool {
	env := expression.DefaultEnv(state)
	for _, prog := range v.progs {
		result, err := expr.Run(prog, env)
		if err != nil {
			return false
		}
		if b, ok := result.(bool); ok && b {
			continue
		}
		// anything other than a boolean true result is considered false
		return false
	}
	return true
}

// Name implements the Handler interface
func (c *View) Name() string { return "view" }

// ServeDNS implements the Handler interface.
func (c *View) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(c.Name(), c.Next, ctx, w, r)
}
