// Package geoip implements a max mind database plugin.
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

// GeoIP is a plugin that add geo location data to the request context by looking up a maxmind
// geoIP2 database, and which data can be later consumed by other middlewares.
type GeoIP struct {
	Next plugin.Handler
	db   db
}

type db struct {
	*geoip2.Reader
	// provides defines the schemas that can be obtained by querying this database, by using
	// bitwise operations.
	provides int
}

const (
	city = 1 << iota
)

var probingIP = net.ParseIP("127.0.0.1")

func newGeoIP(dbPath string) (*GeoIP, error) {
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
	}
	// Query the database to figure out the database type.
	for _, schema := range schemas {
		if err := schema.validate(); err != nil {
			// If we get an InvalidMethodError then we know this database does not provide that schema.
			if _, ok := err.(geoip2.InvalidMethodError); !ok {
				return nil, fmt.Errorf("unexpected failure looking up database %q schema %q: %v", filepath.Base(dbPath), schema.name, err)
			}
		} else {
			db.provides = db.provides | schema.provides
		}
	}

	if db.provides&city == 0 {
		return nil, fmt.Errorf("database does not provide city schema")
	}

	return &GeoIP{db: db}, nil
}

// ServeDNS implements the plugin.Handler interface.
func (g GeoIP) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	return plugin.NextOrFailure(pluginName, g.Next, ctx, w, r)
}

// Metadata implements the metadata.Provider Interface in the metadata plugin, and is used to store
// the data associated with the source IP of every request.
func (g GeoIP) Metadata(ctx context.Context, state request.Request) context.Context {
	srcIP := net.ParseIP(state.IP())

	switch {
	case g.db.provides&city == city:
		data, err := g.db.City(srcIP)
		if err != nil {
			log.Debugf("Setting up metadata failed due to database lookup error: %v", err)
			return ctx
		}
		g.setCityMetadata(ctx, data)
	}
	return ctx
}

// Name implements the Handler interface.
func (g GeoIP) Name() string { return pluginName }
