// Package asnlookup implements ASN lookup using MaxMind GeoLite2 ASN database.
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

// ASNLookup is the CoreDNS plugin for ASN lookup.
type ASNLookup struct {
	Next  plugin.Handler
	db    *geoip2.Reader
	edns0 bool
}

var log = clog.NewWithPlugin(pluginName) // Using clog for logging.

// newASNLookup initializes the plugin with the given database path.
func newASNLookup(dbPath string, edns0 bool) (*ASNLookup, error) {
	reader, err := geoip2.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ASN database file: %v", err)
	}

	return &ASNLookup{db: reader, edns0: edns0}, nil
}

// ServeDNS processes DNS requests.
func (a ASNLookup) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(pluginName, a.Next, ctx, w, r)
}

// Metadata implements the metadata.Provider interface to add ASN information.
func (a ASNLookup) Metadata(ctx context.Context, state request.Request) context.Context {
	srcIP := net.ParseIP(state.IP())

	// Handle EDNS0 Client Subnet if enabled.
	if a.edns0 {
		if o := state.Req.IsEdns0(); o != nil {
			for _, s := range o.Option {
				if e, ok := s.(*dns.EDNS0_SUBNET); ok {
					srcIP = e.Address
					break
				}
			}
		}
	}

	// Lookup ASN from the database.
	record, err := a.db.ASN(srcIP)
	if err != nil {
		log.Debugf("ASN lookup failed for IP %s: %v", srcIP, err)
		return ctx
	}

	// Set ASN metadata.
	metadata.SetValueFunc(ctx, pluginName+"/asn", func() string {
		return fmt.Sprintf("%d", record.AutonomousSystemNumber)
	})
	metadata.SetValueFunc(ctx, pluginName+"/organization", func() string {
		return record.AutonomousSystemOrganization
	})

	return ctx
}

// Name returns the plugin name.
func (a ASNLookup) Name() string { return pluginName }
