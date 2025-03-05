package view

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/expression"
	"github.com/coredns/coredns/request"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/miekg/dns"
)

// View is a plugin that enables configuring expression based advanced routing
type View struct {
	progs    []*vm.Program
	viewName string
	Next     plugin.Handler
}

// Filter implements dnsserver.Viewer.  It returns true if all View rules evaluate to true for the given state.
func (v *View) Filter(ctx context.Context, state *request.Request) bool {
	env := expression.DefaultEnv(ctx, state)
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

// ViewName implements dnsserver.Viewer. It returns the view name
func (v *View) ViewName() string { return v.viewName }

// Name implements the Handler interface
func (*View) Name() string { return "view" }

// ServeDNS implements the Handler interface.
func (v *View) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(v.Name(), v.Next, ctx, w, r)
}
