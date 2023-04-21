// package atlas is a CoreDNS plugin that prints "atlas" to stdout on every packet received.
//
// It serves as an atlas CoreDNS plugin with numerous code comments.
package atlas

import (
	"context"
	"fmt"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/atlas/ent"
	"github.com/coredns/coredns/plugin/atlas/ent/dnsrr"
	"github.com/coredns/coredns/plugin/atlas/ent/dnszone"
	"github.com/coredns/coredns/plugin/atlas/record"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Atlas is an database plugin.
type Atlas struct {
	Next plugin.Handler

	cfg            *Config
	zones          []string
	lastZoneUpdate time.Time
}

// ServeDNS implements the plugin.Handler interface. This method gets called when atlas is used
// in a Server.
func (a *Atlas) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	log.Infof("Atlas ServeDNS")
	req := request.Request{W: w, Req: r}

	reqName := req.Name()
	reqType := req.QType()

	client, err := a.getAtlasClient()
	if err != nil {
		return a.errorResponse(req, dns.RcodeServerFailure, err)
	}
	defer client.Close()

	log.Info("question name: ", reqName)
	log.Infof("question type: %v => %v", reqType, req.Type())

	if time.Since(a.lastZoneUpdate) > a.cfg.zoneUpdateTime {
		log.Info("++++++ LOADING ZONES +++++++")
		err := a.loadZones(ctx, client)
		if err != nil {
			return a.errorResponse(req, dns.RcodeServerFailure, err)
		}
		a.lastZoneUpdate = time.Now()
	}

	reqZone := plugin.Zones(a.zones).Matches(reqName)
	if reqZone == "" {
		return plugin.NextOrFailure(a.Name(), a.Next, ctx, w, r)
	}

	// handle soa record
	if reqType == dns.TypeSOA {
		soaRec, err := a.getSOARecord(ctx, client, reqZone)
		if err != nil {
			return a.errorResponse(req, dns.RcodeServerFailure, err)
		}
		log.Info(soaRec)

	}

	// TODO:(jproxx) do something with with the rrs
	rrs, err := a.getRRecords(ctx, client, reqName, reqType)
	if err != nil {
		return a.errorResponse(req, dns.RcodeServerFailure, err)
	}
	log.Info("rec ", len(rrs))

	// Wrap.
	pw := NewResponsePrinter(w)

	if len(rrs) == 0 {
		// TODO: nothing found - check what we have to answer
		// TODO: some plugins are sending SOA rr...
		plugin.NextOrFailure(a.Name(), a.Next, ctx, pw, r)
	}

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

func (a *Atlas) loadZones(ctx context.Context, client *ent.Client) error {
	zones := []string{}

	if client == nil {
		return fmt.Errorf("atlas client error")
	}

	records, err := client.DnsZone.
		Query().
		Select(dnszone.FieldName).
		Where(dnszone.Activated(true)).
		Order(ent.Asc(dnszone.FieldName)).
		All(ctx)
	if err != nil {
		return err
	}

	log.Infof("loadZones found: %v zone(s)", len(records))

	for _, zone := range records {
		zones = append(zones, zone.Name)
	}

	a.zones = zones

	return nil
}

func (a Atlas) getSOARecord(ctx context.Context, client *ent.Client, zone string) (rrs []dns.RR, err error) {
	rrs = make([]dns.RR, 0)
	if client == nil {
		return rrs, fmt.Errorf("atlas client error")
	}

	soaRec, err := client.DnsZone.Query().
		Where(
			dnszone.Activated(true),
			dnszone.NameEQ(zone),
		).
		First(ctx)

	if err != nil {
		return rrs, err
	}

	rec := &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   soaRec.Name,
			Rrtype: soaRec.Rrtype,
			Class:  soaRec.Class,
			Ttl:    soaRec.TTL,
		},
		Ns:      soaRec.Ns,
		Mbox:    soaRec.Mbox,
		Serial:  soaRec.Serial,
		Refresh: soaRec.Refresh,
		Retry:   soaRec.Retry,
		Expire:  soaRec.Expire,
		Minttl:  soaRec.Minttl,
	}

	return []dns.RR{rec}, nil
}

func (a Atlas) getRRecords(ctx context.Context, client *ent.Client, reqName string, reqQType uint16) (rrs []dns.RR, err error) {
	rrs = make([]dns.RR, 0)
	if client == nil {
		return rrs, fmt.Errorf("atlas client error")
	}

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
		).
		Order(ent.Asc(
			dnsrr.FieldName,
			dnsrr.FieldRrtype,
		)).
		All(ctx)
	if err != nil {
		return rrs, err
	}

	for _, r := range records {
		rec, err := record.From(r)
		if err != nil {
			log.Error(err)
			return rrs, err
		}
		rrs = append(rrs, rec)
	}

	return rrs, nil
}

func (handler *Atlas) errorResponse(state request.Request, rCode int, err error) (int, error) {
	m := new(dns.Msg)
	m.SetRcode(state.Req, rCode)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, false, true

	state.SizeAndDo(m)
	_ = state.W.WriteMsg(m)
	// Return success as the rCode to signal we have written to the client.
	return dns.RcodeSuccess, err
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
