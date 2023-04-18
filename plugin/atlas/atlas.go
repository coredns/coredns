// package atlas is a CoreDNS plugin that prints "atlas" to stdout on every packet received.
//
// It serves as an atlas CoreDNS plugin with numerous code comments.
package atlas

import (
	"context"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/atlas/ent"
	"github.com/coredns/coredns/plugin/atlas/ent/dnsrr"
	"github.com/coredns/coredns/plugin/atlas/record"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Atlas is an database plugin.
type Atlas struct {
	Next  plugin.Handler
	Zones []string
	cfg   Config
}

// ServeDNS implements the plugin.Handler interface. This method gets called when atlas is used
// in a Server.
func (a Atlas) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	req := request.Request{W: w, Req: r}

	questName := req.Name()
	questType := req.QType()
	zone := dns.Fqdn(questName)

	log.Info("question name: ", questName)
	log.Infof("question type: %v => %v", questType, req.Type())
	log.Infof("zone: %v", zone)

	a.getRRecords(ctx, questName, questType)

	// Wrap.
	pw := NewResponsePrinter(w)

	// Export metric with the server label set to the current server handling the request.
	requestCount.WithLabelValues(metrics.WithServer(ctx)).Inc()

	// Call next plugin (if any).
	return plugin.NextOrFailure(a.Name(), a.Next, ctx, pw, r)
}

// Name implements the Handler interface.
func (a Atlas) Name() string { return plgName }

func (a Atlas) getAtlasClient() (client *ent.Client, err error) {
	dsn, err := a.cfg.GetDsn()
	if err != nil {
		return nil, err
	}

	client, err = OpenAtlasDB(dsn)
	if err != nil {
		return nil, err
	}

	if a.cfg.debug {
		client = client.Debug()
	}
	return
}

func (a Atlas) getRRecords(ctx context.Context, reqName string, reqQType uint16) (rrs []dns.RR, err error) {
	client, err := a.getAtlasClient()
	if err != nil {
		return rrs, err
	}
	defer client.Close()

	records, err := client.DnsRR.Query().
		Select(
			dnsrr.FieldName,
			dnsrr.FieldClass,
			dnsrr.FieldRrtype,
			dnsrr.FieldRrdata,
			dnsrr.FieldTTL,
		).
		Where(
			dnsrr.NameEQ(reqName),
			dnsrr.RrtypeEQ(reqQType),
			dnsrr.ActivatedEQ(true), // we serve only activated records
		).All(ctx)
	if err != nil {
		return rrs, err
	}

	for _, r := range records {
		record.From(r)
	}

	return rrs, nil
}

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
