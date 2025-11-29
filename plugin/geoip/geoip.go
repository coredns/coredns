// Package geoip implements an MMDB database plugin for geo/network IP lookups.
package geoip

import (
	"context"
	"fmt"
	"net"
	"path/filepath"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"github.com/oschwald/geoip2-golang"
)

var log = clog.NewWithPlugin(pluginName)

// GeoIP is a plugin that adds geo location and network data to the request context by looking up
// an MMDB format database, and which data can be later consumed by other middlewares.
type GeoIP struct {
	Next  plugin.Handler
	db    db
	edns0 bool
}

type db struct {
	*geoip2.Reader
	// provides defines the schemas that can be obtained by querying this database, by using
	// bitwise operations.
	provides int
}

const (
	city = 1 << iota
	asn
)

var probingIP = net.ParseIP("127.0.0.1")

func newGeoIP(dbPath string, edns0 bool) (*GeoIP, error) {
	reader, err := geoip2.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database file: %v", err)
	}
	db := db{Reader: reader}
	schemas := []struct {
		provides int
		name     string
		validate func() error
	}{
		{name: "city", provides: city, validate: func() error { _, err := reader.City(probingIP); return err }},
		{name: "asn", provides: asn, validate: func() error { _, err := reader.ASN(probingIP); return err }},
	}
	// Query the database to figure out the database type.
	for _, schema := range schemas {
		if err := schema.validate(); err != nil {
			// If we get an InvalidMethodError then we know this database does not provide that schema.
			if _, ok := err.(geoip2.InvalidMethodError); !ok {
				return nil, fmt.Errorf("unexpected failure looking up database %q schema %q: %v", filepath.Base(dbPath), schema.name, err)
			}
		} else {
			db.provides |= schema.provides
		}
	}

	if db.provides == 0 {
		return nil, fmt.Errorf("database does not provide any supported schema (city, asn)")
	}

	return &GeoIP{db: db, edns0: edns0}, nil
}

// ServeDNS implements the plugin.Handler interface.
func (g GeoIP) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(pluginName, g.Next, ctx, w, r)
}

// Metadata implements the metadata.Provider Interface in the metadata plugin, and is used to store
// the data associated with the source IP of every request.
func (g GeoIP) Metadata(ctx context.Context, state request.Request) context.Context {
	srcIP := net.ParseIP(state.IP())

	if g.edns0 {
		if o := state.Req.IsEdns0(); o != nil {
			for _, s := range o.Option {
				if e, ok := s.(*dns.EDNS0_SUBNET); ok {
					srcIP = e.Address
					break
				}
			}
		}
	}

	if g.db.provides&city != 0 {
		data, err := g.db.City(srcIP)
		if err != nil {
			log.Debugf("Setting up city metadata failed due to database lookup error: %v", err)
		} else {
			g.setCityMetadata(ctx, data)
		}
	}
	if g.db.provides&asn != 0 {
		data, err := g.db.ASN(srcIP)
		if err != nil {
			log.Debugf("Setting up asn metadata failed due to database lookup error: %v", err)
		} else {
			g.setASNMetadata(ctx, data)
		}
	}
	return ctx
}

// Name implements the Handler interface.
func (g GeoIP) Name() string { return pluginName }
