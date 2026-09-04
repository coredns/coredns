// Package dynupdate implements RFC 2136 dynamic updates for a file-backed
// authoritative zone.
//
// The seed file is read at startup and is never modified. Updates are kept in
// memory until the process restarts. The plugin owns its zone so that normal
// queries and AXFR see the same atomically replaced snapshot.
package dynupdate

import (
	"context"
	"sync"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/plugin/transfer"

	"github.com/miekg/dns"
)

const pluginName = "dynupdate"

var (
	_ plugin.Handler      = (*DynUpdate)(nil)
	_ transfer.Transferer = (*DynUpdate)(nil)
)

// DynUpdate serves one authoritative zone and accepts RFC 2136 UPDATE
// messages for it.
type DynUpdate struct {
	Next plugin.Handler

	// Zone is the canonical, fully-qualified origin served by this instance.
	Zone string

	// Xfer is populated when the transfer plugin is configured in the same
	// server block. It is used for best-effort NOTIFY after a committed update.
	Xfer *transfer.Transfer

	permissions []permission

	mu      sync.RWMutex
	records []dns.RR
	view    *file.File
}

// ServeDNS implements the plugin.Handler interface.
func (d *DynUpdate) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if r.Opcode == dns.OpcodeUpdate {
		return d.serveUpdate(ctx, w, r)
	}

	d.mu.RLock()
	view := d.view
	d.mu.RUnlock()
	if view == nil {
		return dns.RcodeServerFailure, nil
	}

	return view.ServeDNS(ctx, w, r)
}

// Transfer implements transfer.Transferer. The current immutable view is
// used, so a transfer observes either the old or the new zone generation.
func (d *DynUpdate) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	zone = canonicalName(zone)
	if zone != d.Zone {
		return nil, transfer.ErrNotAuthoritative
	}

	d.mu.RLock()
	view := d.view
	d.mu.RUnlock()
	if view == nil {
		return nil, transfer.ErrNotAuthoritative
	}
	return view.Transfer(zone, serial)
}

// Name implements the plugin.Handler interface.
func (d *DynUpdate) Name() string { return pluginName }

// build creates the read and transfer view for a record snapshot. file.Zone
// already contains CoreDNS's authoritative lookup, wildcard, delegation, and
// DNSSEC response behavior, so this plugin does not duplicate those rules.
func (d *DynUpdate) build(records []dns.RR) (*file.File, error) {
	if err := validateRecords(records, d.Zone); err != nil {
		return nil, err
	}

	z := file.NewZone(d.Zone, "")
	z.Upstream = upstream.New()
	for _, rr := range records {
		if err := z.Insert(dns.Copy(rr)); err != nil {
			return nil, err
		}
	}
	return &file.File{
		Next: d.Next,
		Zones: file.Zones{
			Z:     map[string]*file.Zone{d.Zone: z},
			Names: []string{d.Zone},
		},
	}, nil
}

// install swaps a fully built view. The caller must hold d.mu for writing.
func (d *DynUpdate) install(records []dns.RR, view *file.File) {
	d.records = records
	d.view = view
}
