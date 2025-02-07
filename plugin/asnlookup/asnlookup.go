package asnlookup

import (
	"context"
	"fmt"
	"net"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metadata"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"

	"github.com/oschwald/geoip2-golang"
)

const pluginName = "asnlookup"

type ASNLookup struct {
	Next plugin.Handler
	db   *geoip2.Reader
}

var log = clog.NewWithPlugin(pluginName)

// NewASNLookup initializes the plugin with the given database path.
func NewASNLookup(dbPath string) (*ASNLookup, error) {
	reader, err := geoip2.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ASN database file: %v", err)
	}

	return &ASNLookup{db: reader}, nil
}

// ServeDNS processes DNS requests.
func (a ASNLookup) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	log.Infof("Received DNS query for name: %s from IP: %s", state.Name(), state.IP())
	return plugin.NextOrFailure(pluginName, a.Next, ctx, w, r)
}

// Metadata implements the metadata.Provider interface to add ASN information.
func (a ASNLookup) Metadata(ctx context.Context, state request.Request) context.Context {
	srcIP := net.ParseIP(state.IP())

	// Lookup ASN from the database.
	record, err := a.db.ASN(srcIP)
	if err != nil {
		log.Errorf("ASN lookup failed for IP %s: %v", srcIP, err)
		return ctx
	}

	log.Infof("ASN lookup successful for IP %s: ASN=%d, Org=%s",
		srcIP, record.AutonomousSystemNumber, record.AutonomousSystemOrganization)

	// Set ASN metadata in context using metadata.SetValueFunc.
	metadata.SetValueFunc(ctx, pluginName+"/asn", func() string {
		return fmt.Sprintf("%d", record.AutonomousSystemNumber)
	})
	metadata.SetValueFunc(ctx, pluginName+"/organization", func() string {
		return record.AutonomousSystemOrganization
	})

	return ctx
}

func (a ASNLookup) Name() string { return pluginName }
