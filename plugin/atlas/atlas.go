// package atlas is a CoreDNS plugin that prints "atlas" to stdout on every packet received.
//
// It serves as an atlas CoreDNS plugin with numerous code comments.
package atlas

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/atlas/ent"
	"github.com/coredns/coredns/plugin/metrics"

	"github.com/miekg/dns"
)

// Atlas is an database plugin.
type Atlas struct {
	Next   plugin.Handler
	Zones  []string
	cfg    Config
	client *ent.Client
}

// ServeDNS implements the plugin.Handler interface. This method gets called when atlas is used
// in a Server.
func (a Atlas) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	log.Info("Atlas ServeDNS context\n", ctx)

	// Wrap.
	pw := NewResponsePrinter(w)

	// Export metric with the server label set to the current server handling the request.
	requestCount.WithLabelValues(metrics.WithServer(ctx)).Inc()

	// Call next plugin (if any).
	return plugin.NextOrFailure(a.Name(), a.Next, ctx, pw, r)
}

// Name implements the Handler interface.
func (a Atlas) Name() string { return plgName }

// ResponsePrinter wrap a dns.ResponseWriter and will write atlas to standard output when WriteMsg is called.
type ResponsePrinter struct {
	dns.ResponseWriter
}

// NewResponsePrinter returns ResponseWriter.
func NewResponsePrinter(w dns.ResponseWriter) *ResponsePrinter {
	return &ResponsePrinter{ResponseWriter: w}
}

// WriteMsg calls the underlying ResponseWriter's WriteMsg method and prints "atlas" to standard output.
func (r *ResponsePrinter) WriteMsg(res *dns.Msg) error {
	log.Info(plgName)
	return r.ResponseWriter.WriteMsg(res)
}
