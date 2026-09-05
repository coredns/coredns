// Package dynupdate implements RFC 2136 dynamic updates for a file-backed
// authoritative zone.
//
// The seed file is never modified. An optional local database makes accepted
// updates durable. Queries and AXFR see the same atomically replaced snapshot.
package dynupdate

import (
	"context"
	"fmt"
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
	limits      limits
	seed        string
	database    string

	mu            sync.RWMutex
	records       []dns.RR
	view          *file.File
	store         *zoneStore
	closed        bool
	notifyPending bool
	notifyRunning bool
}

// ServeDNS implements the plugin.Handler interface.
func (d *DynUpdate) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if r.Opcode == dns.OpcodeUpdate {
		return d.serveUpdate(ctx, w, r)
	}
	if len(r.Question) == 1 && !inZone(d.Zone, r.Question[0].Name) {
		return plugin.NextOrFailure(d.Name(), d.Next, ctx, w, r)
	}
	view, err := d.snapshot()
	if err != nil {
		return dns.RcodeServerFailure, err
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

	view, err := d.snapshot()
	if err != nil {
		return nil, err
	}
	return view.Transfer(zone, serial)
}

// Name implements the plugin.Handler interface.
func (d *DynUpdate) Name() string { return pluginName }

// CacheBypassZones prevents caching of mutable data without bypassing other
// middleware between cache and this authoritative backend.
func (d *DynUpdate) CacheBypassZones() []string { return []string{d.Zone} }

func (d *DynUpdate) snapshot() (*file.File, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.ensureStore(); err != nil {
		return nil, err
	}
	view := d.view
	if d.store != nil {
		d.store.mu.RLock()
		view = d.store.view
		d.store.mu.RUnlock()
	}
	if view == nil {
		return nil, fmt.Errorf("zone %q has no snapshot", d.Zone)
	}
	copyView := *view
	copyView.Next = d.Next
	return &copyView, nil
}

// build creates the read and transfer view for a record snapshot. file.Zone
// already contains CoreDNS's authoritative lookup, wildcard, delegation, and
// DNSSEC response behavior, so this plugin does not duplicate those rules.
func (d *DynUpdate) build(records []dns.RR) (*file.File, error) {
	if err := d.limits.check(records); err != nil {
		return nil, err
	}
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
