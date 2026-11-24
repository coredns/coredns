package view

import (
	"context"
	"net"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/expression"
	ptest "github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/miekg/dns"
)

func TestFilter_NoPrograms(t *testing.T) {
	v := &View{viewName: "test"}
	st := makeState(t, "example.com.")
	if !v.Filter(context.Background(), st) {
		t.Fatalf("expected true when no programs are configured")
	}
}

func TestFilter_ErrorPaths(t *testing.T) {
	st := makeState(t, "example.com.")

	tests := []struct {
		expr     string
		expected bool
	}{
		{"name() == 'example.com.'", true},
		{"name() == 'notexample.com.'", false},
		{"1", false},
		{"incidr('invalid', '1.2.3.0/24')", false},
	}

	for i, tc := range tests {
		v := &View{progs: []*vm.Program{compileExpr(t, tc.expr)}}
		got := v.Filter(context.Background(), st)
		if got != tc.expected {
			t.Fatalf("case %d expr %q: expected %v, got %v", i, tc.expr, tc.expected, got)
		}
	}
}

func TestView_Names(t *testing.T) {
	v := &View{viewName: "v1"}
	if v.ViewName() != "v1" {
		t.Fatalf("ViewName() expected %q, got %q", "v1", v.ViewName())
	}
	if v.Name() != "view" {
		t.Fatalf("Name() expected %q, got %q", "view", v.Name())
	}
}

func compileExpr(t *testing.T, e string) *vm.Program {
	t.Helper()
	prog, err := expr.Compile(e, expr.Env(expression.DefaultEnv(context.Background(), nil)), expr.DisableBuiltin("type"))
	if err != nil {
		t.Fatalf("compile failed for %q: %v", e, err)
	}
	return prog
}

func makeState(t *testing.T, qname string) *request.Request {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(qname), dns.TypeA)
	w := &ptest.ResponseWriter{}
	return &request.Request{W: w, Req: m}
}

type scopedAddrResponseWriter struct {
	ptest.ResponseWriter
	remote net.Addr
}

func (w *scopedAddrResponseWriter) RemoteAddr() net.Addr { return w.remote }

func TestFilter_InCidrLinkLocalClient(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	st := &request.Request{
		W: &scopedAddrResponseWriter{
			remote: &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: 53000, Zone: "eth0"},
		},
		Req: m,
	}

	matchView := &View{progs: []*vm.Program{compileExpr(t, "incidr(client_ip(), 'fe80::/10')")}}
	if !matchView.Filter(context.Background(), st) {
		t.Fatalf("expected view to match link-local client %q", st.IP())
	}

	missView := &View{progs: []*vm.Program{compileExpr(t, "incidr(client_ip(), '2001:db8::/32')")}}
	if missView.Filter(context.Background(), st) {
		t.Fatalf("expected view not to match link-local client %q", st.IP())
	}
}
