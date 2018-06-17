// Package file implements a file backend.
package file

import (
	"context"
	"fmt"
	"io"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"errors"

	"github.com/coredns/coredns/dynapi"
	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("file")

type (
	// File is the plugin that reads zone data from disk.
	File struct {
		Next  plugin.Handler
		Zones Zones
	}

	// Zones maps zone names to a *Zone.
	Zones struct {
		Z     map[string]*Zone // A map mapping zone (origin) to the Zone's data
		Names []string         // All the keys from the map Z as a string slice.
	}
)

// Upsert implements the `dynapi.Writable` interface.
// It either creates or updates an existing dns resource record
// in the zone specified in `request.Zone` if existing.
// Also the file-reload mutex is locked (and later released) to make
// sure no concurrent unpredictable order of creation/deletion of dns resource record happens.
func (f File) Upsert(request *dynapi.Request) error {
	zone, ok := f.Zones.Z[request.Zone]
	if !ok {
		return errors.New("zone not found")
	}

	resourceRecord, err := request.ToDNSRecordResource()
	if err != nil {
		return err
	}

	dnsRRs := f.findRecordsInZoneByName(zone, resourceRecord.Header().Name)

	zone.reloadMu.Lock()
	defer zone.reloadMu.Unlock()

	if dnsRRs != nil {
		for _, dnsRR := range dnsRRs {
			// TODO: Discuss adding `error` as
			// return type to `zone.Delete`.
			zone.Delete(dnsRR)
		}
	}

	return zone.Insert(resourceRecord)
}

// Create implements the `dynapi.Writable` interface.
// It creates an existing dns resource record
// in the zone specified in `request.Zone` if existing.
// Also the file-reload mutex is locked (and later released) to make
// sure no concurrent unpredictable order of creation/deletion of dns resource record happens.
func (f File) Create(request *dynapi.Request) error {
	zone, ok := f.Zones.Z[request.Zone]
	if !ok {
		return errors.New("zone not found")
	}

	dnsRR, err := request.ToDNSRecordResource()
	if err != nil {
		return err
	}

	zone.reloadMu.Lock()
	defer zone.reloadMu.Unlock()
	return zone.Insert(dnsRR)
}

// Delete implements the `dynapi.Writable` interface.
// It deletes an existing dns resource record
// in the zone specified in `request.Zone` if both zone and record are existing.
// Also the file-reload mutex is locked (and later released) to make
// sure no concurrent unpredictable order of creation/deletion of dns resource record happens.
func (f File) Delete(request *dynapi.Request) error {
	zone, ok := f.Zones.Z[request.Zone]
	if !ok {
		return errors.New("zone not found")
	}

	resourceRecord, err := request.ToDNSRecordResource()
	if err != nil {
		return err
	}

	zone.reloadMu.Lock()
	defer zone.reloadMu.Unlock()

	// TODO: Discuss adding `error` as
	// return type to `zone.Delete`.
	zone.Delete(resourceRecord)
	return nil
}

// Update implements the `dynapi.Writable` interface.
// It updates an existing dns resource record
// in the zone specified in `request.Zone` if both zone and record are existing by name.
// Also the file-reload mutex is locked (and later released) to make
// sure no concurrent unpredictable order of creation/deletion of dns resource record happens.
func (f File) Update(request *dynapi.Request) error {
	zone, ok := f.Zones.Z[request.Zone]
	if !ok {
		return errors.New("zone not found")
	}

	resourceRecord, err := request.ToDNSRecordResource()
	if err != nil {
		return err
	}

	dnsRRs := f.findRecordsInZoneByName(zone, resourceRecord.Header().Name)
	if dnsRRs == nil {
		return errors.New("no dns resource record found")
	}

	zone.reloadMu.Lock()
	defer zone.reloadMu.Unlock()
	for _, dnsRR := range dnsRRs {
		// TODO: Discuss adding `error` as
		// return type to `zone.Delete`.
		zone.Delete(dnsRR)
	}

	return zone.Insert(resourceRecord)
}

// Exists implements the `dynapi.Writable` interface.
// It checks whether a dns resource record exists in the specified
// zone by `dynapi.Request` if zone is existing.
func (f File) Exists(request *dynapi.Request) bool {
	zone, ok := f.Zones.Z[request.Zone]
	if !ok {
		return false
	}

	resourceRecord, err := request.ToDNSRecordResource()
	if err != nil {
		return false
	}

	// TODO: Perhaps `zone.searchGlue(name, do)`
	// might be more appropriate to use.
	for _, r := range zone.All() {
		if r.String() == resourceRecord.String() {
			return true
		}
	}

	return false
}

func (f File) findRecordsInZoneByName(zone *Zone, name string) []dns.RR {
	var records []dns.RR

	for _, r := range zone.All() {
		rrType := r.Header().Rrtype
		if (rrType == dns.TypeA || rrType == dns.TypeAAAA) && r.Header().Name == name {
			records = append(records, r)
		}
	}

	return records
}

// ExistsByName implements the `dynapi.Writable` interface.
// It checks whether a dns resource record exists by name in the specified
// zone by `dynapi.Request` if zone is existing.
func (f File) ExistsByName(request *dynapi.Request) bool {
	zone, ok := f.Zones.Z[request.Zone]
	if !ok {
		return false
	}

	resourceRecord, err := request.ToDNSRecordResource()
	if err != nil {
		return false
	}

	return f.findRecordsInZoneByName(zone, resourceRecord.Header().Name) != nil
}

// GetZones returns all zones handled by the `file` plugin.
func (f File) GetZones() []string {
	return f.Zones.Names
}

// ServeDNS implements the plugin.Handle interface.
func (f File) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r, Context: ctx}

	qname := state.Name()
	// TODO(miek): match the qname better in the map
	zone := plugin.Zones(f.Zones.Names).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(f.Name(), f.Next, ctx, w, r)
	}

	z, ok := f.Zones.Z[zone]
	if !ok || z == nil {
		return dns.RcodeServerFailure, nil
	}

	// This is only for when we are a secondary zones.
	if r.Opcode == dns.OpcodeNotify {
		if z.isNotify(state) {
			m := new(dns.Msg)
			m.SetReply(r)
			m.Authoritative, m.RecursionAvailable = true, true
			state.SizeAndDo(m)
			w.WriteMsg(m)

			log.Infof("Notify from %s for %s: checking transfer", state.IP(), zone)
			ok, err := z.shouldTransfer()
			if ok {
				z.TransferIn()
			} else {
				log.Infof("Notify from %s for %s: no serial increase seen", state.IP(), zone)
			}
			if err != nil {
				log.Warningf("Notify from %s for %s: failed primary check: %s", state.IP(), zone, err)
			}
			return dns.RcodeSuccess, nil
		}
		log.Infof("Dropping notify from %s for %s", state.IP(), zone)
		return dns.RcodeSuccess, nil
	}

	if z.Expired != nil && *z.Expired {
		log.Errorf("Zone %s is expired", zone)
		return dns.RcodeServerFailure, nil
	}

	if state.QType() == dns.TypeAXFR || state.QType() == dns.TypeIXFR {
		xfr := Xfr{z}
		return xfr.ServeDNS(ctx, w, r)
	}

	answer, ns, extra, result := z.Lookup(state, qname)

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable = true, true
	m.Answer, m.Ns, m.Extra = answer, ns, extra

	switch result {
	case Success:
	case NoData:
	case NameError:
		m.Rcode = dns.RcodeNameError
	case Delegation:
		m.Authoritative = false
	case ServerFailure:
		return dns.RcodeServerFailure, nil
	}

	state.SizeAndDo(m)
	m, _ = state.Scrub(m)
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// Name implements the Handler interface.
func (f File) Name() string { return "file" }

type serialErr struct {
	err    string
	zone   string
	origin string
	serial int64
}

func (s *serialErr) Error() string {
	return fmt.Sprintf("%s for origin %s in file %s, with serial %d", s.err, s.origin, s.zone, s.serial)
}

// Parse parses the zone in filename and returns a new Zone or an error.
// If serial >= 0 it will reload the zone, if the SOA hasn't changed
// it returns an error indicating nothing was read.
func Parse(f io.Reader, origin, fileName string, serial int64) (*Zone, error) {
	tokens := dns.ParseZone(f, dns.Fqdn(origin), fileName)
	z := NewZone(origin, fileName)
	seenSOA := false
	for x := range tokens {
		if x.Error != nil {
			return nil, x.Error
		}

		if !seenSOA && serial >= 0 {
			if s, ok := x.RR.(*dns.SOA); ok {
				if s.Serial == uint32(serial) { // same serial
					return nil, &serialErr{err: "no change in SOA serial", origin: origin, zone: fileName, serial: serial}
				}
				seenSOA = true
			}
		}

		if err := z.Insert(x.RR); err != nil {
			return nil, err
		}
	}
	if !seenSOA {
		return nil, fmt.Errorf("file %q has no SOA record", fileName)
	}

	return z, nil
}
