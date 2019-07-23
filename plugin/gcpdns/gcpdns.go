// Package gcpdns implements a plugin that returns DNS Resource Records
// from GCP Cloud DNS.
package gcpdns

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/gcpdns/api"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"
	dnslib "github.com/miekg/dns"
	"github.com/pkg/errors"
)

// gcpDNSPlugin is a plugin that returns DNS Resource Records from GCP Cloud DNS.
type gcpDNSPlugin struct {
	name string

	Next plugin.Handler
	Fall fall.F

	zoneNames []string
	client    *api.Service
	upstream  *upstream.Upstream

	zMu   sync.RWMutex
	zones zones
}

type zoneID struct {
	project string
	name    string
}

func (zid zoneID) String() string {
	return zid.project + "/" + zid.name
}

type zone struct {
	id   zoneID
	zone *file.Zone
	dns  string
}

type zones map[string][]*zone

// newGcpDNSHandler reads from the keys map which uses domain names as its key
// and hosted zone name lists as its values, validates that each domain name/zone
// name pair does exist, and returns a new *gcpDNSPlugin. In addition to this, upstream
// is passed for doing recursive queries against CNAMEs.
// Returns error if it cannot verify any given domain name/zone name pair.
func newGcpDNSHandler(ctx context.Context, name string, s *api.Service, keys map[string][]zoneID, up *upstream.Upstream) (*gcpDNSPlugin, error) {
	zones := make(map[string][]*zone, len(keys))
	zoneNames := make([]string, 0, len(keys))
	for dn, zids := range keys {
		for _, zid := range zids {
			_, err := s.ManagedZones.Get(zid.project, zid.name).Context(ctx).Do()
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to inspect zone %s in %s", zid.name, zid.project)
			}

			if _, ok := zones[dn]; !ok {
				zoneNames = append(zoneNames, dn)
			}
			zones[dn] = append(zones[dn], &zone{id: zid, dns: dn, zone: file.NewZone(dn, "")})
		}
	}
	return &gcpDNSPlugin{
		name:      name,
		client:    s,
		zoneNames: zoneNames,
		zones:     zones,
		upstream:  up,
	}, nil
}

// startup executes first update, spins up an update forever-loop.
// Returns error if first update fails.
func (h *gcpDNSPlugin) startup(ctx context.Context) error {
	if err := h.updateZones(ctx); err != nil {
		return err
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Minute):
				if err := h.updateZones(ctx); err != nil && ctx.Err() == nil /* Don't log error if ctx expired. */ {
					log.Errorf("[%s] Failed to update zones: %v", h.name, err)
				}
			}
		}
	}()
	return nil
}

// ServeDNS implements the plugin.Handler.ServeDNS.
func (h *gcpDNSPlugin) ServeDNS(ctx context.Context, w dnslib.ResponseWriter, r *dnslib.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	qname := state.Name()

	zName := plugin.Zones(h.zoneNames).Matches(qname)
	if zName == "" {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}
	z, ok := h.zones[zName]
	if !ok || z == nil {
		return dnslib.RcodeServerFailure, nil
	}

	m := new(dnslib.Msg)
	m.SetReply(r)
	m.Authoritative = true
	var result file.Result
	for _, hostedZone := range z {
		h.zMu.RLock()
		m.Answer, m.Ns, m.Extra, result = hostedZone.zone.Lookup(ctx, state, qname)
		h.zMu.RUnlock()

		// Take the answer if it's non-empty OR if there is another
		// record type exists for this name (NODATA).
		if len(m.Answer) != 0 || result == file.NoData {
			break
		}
	}

	if len(m.Answer) == 0 && result != file.NoData && h.Fall.Through(qname) {
		log.Debugf("[%s] falling through for %s", h.name, qname)
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	log.Debugf("[%s] providing answer for %s", h.name, qname)

	switch result {
	case file.Success:
	case file.NoData:
	case file.NameError:
		m.Rcode = dnslib.RcodeNameError
	case file.Delegation:
		m.Authoritative = false
	case file.ServerFailure:
		return dnslib.RcodeServerFailure, nil
	}

	if err := w.WriteMsg(m); err != nil {
		return dnslib.RcodeServerFailure, nil
	}
	return dnslib.RcodeSuccess, nil
}

// updateZones re-queries resource record sets for each zone and updates the
// zone object.
// Returns error if any zones error'ed out, but waits for other zones to
// complete first.
func (h *gcpDNSPlugin) updateZones(ctx context.Context) error {
	errc := make(chan error)
	defer close(errc)
	for name, zones := range h.zones {
		go func(name string, zones []*zone) {
			var err error
			defer func() {
				errc <- err
			}()

			for i, zone := range zones {
				nz := file.NewZone(name, "")
				nz.Upstream = h.upstream

				rrs, err := h.client.ResourceRecordSets.List(zone.id.project, zone.id.name).Context(ctx).Do()
				if err != nil {
					return
				}

				for _, rr := range rrs.Rrsets {
					// Assemble RFC 1035 conforming record to pass into dns scanner.
					for _, z := range rr.Rrdatas {
						rfc1035 := fmt.Sprintf("%s %d IN %s %s", rr.Name, rr.Ttl, rr.Type, z)
						log.Debugf("[%s] Adding %s", h.name, rfc1035)
						r, err := dnslib.NewRR(rfc1035)
						if err != nil {
							log.Errorf("[%s] failed to parse resource record: %v", h.name, err)
						} else {
							if err := nz.Insert(r); err != nil {
								log.Errorf("[%s] failed to insert resource record: %v", h.name, err)
							}
						}
					}
				}

				h.zMu.Lock()
				(*zones[i]).zone = nz
				h.zMu.Unlock()
			}

		}(name, zones)
	}
	// Collect errors (if any). This will also sync on all zones updates
	// completion.
	var errs []string
	for i := 0; i < len(h.zones); i++ {
		err := <-errc
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) != 0 {
		return fmt.Errorf("[%s] errors updating zones: %v", h.name, errs)
	}
	return nil
}

// Name implements plugin.Handler.Name.
func (h *gcpDNSPlugin) Name() string { return "gcpdns" }
