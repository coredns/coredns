package dynupdate

import (
	"context"
	"fmt"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/tsig"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin(pluginName)

func (d *DynUpdate) serveUpdate(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if len(r.Question) != 1 || r.Question[0].Qtype != dns.TypeSOA || r.Question[0].Qclass != dns.ClassINET {
		return d.reply(w, r, dns.RcodeFormatError)
	}

	zone := canonicalName(r.Question[0].Name)
	if zone != d.Zone {
		d.mu.RLock()
		next := d.Next
		d.mu.RUnlock()
		if next != nil {
			// A server block may contain several authoritative handlers. Let
			// the handler that owns another zone process this UPDATE.
			return plugin.NextOrFailure(d.Name(), next, ctx, w, r)
		}
		return d.reply(w, r, dns.RcodeNotAuth)
	}

	var key string
	var ok bool
	if ctx != nil {
		key, ok = tsig.ValidatedKeyName(ctx)
	}
	if !ok {
		log.Debugf("refusing UPDATE for %s without a validated TSIG identity", zone)
		return d.reply(w, r, dns.RcodeRefused)
	}

	rcode, err := d.applyUpdate(key, r.Answer, r.Ns)
	if err != nil {
		log.Errorf("UPDATE for %s failed: %v", zone, err)
	}
	return d.reply(w, r, rcode)
}

// applyUpdate performs one complete RFC 2136 transaction. It returns only
// after either the old snapshot is unchanged or a fully built new snapshot is
// installed. The caller must not hold d.mu.
func (d *DynUpdate) applyUpdate(key string, prerequisites, updates []dns.RR) (int, error) {
	d.mu.Lock()
	changed, rcode, err := d.updateLocked(key, prerequisites, updates)
	if changed && d.Xfer != nil {
		d.notifyPending = true
		if !d.notifyRunning {
			d.notifyRunning = true
			go d.notify()
		}
	}
	d.mu.Unlock()
	return rcode, err
}

// Coalesce bursts and keep at most one NOTIFY operation in flight per instance.
func (d *DynUpdate) notify() {
	for {
		d.mu.Lock()
		if d.closed || !d.notifyPending {
			d.notifyRunning = false
			d.mu.Unlock()
			return
		}
		d.notifyPending = false
		xfer, zone := d.Xfer, d.Zone
		d.mu.Unlock()
		if err := xfer.Notify(zone); err != nil {
			log.Warningf("NOTIFY for %s after UPDATE failed: %v", zone, err)
		}
	}
}

func (d *DynUpdate) updateLocked(key string, prerequisites, updates []dns.RR) (bool, int, error) {
	// Authenticate and identify the key before doing semantic work on the
	// request. An unknown valid TSIG must not be able to probe zone state.
	if !d.configuredKey(key) {
		return false, dns.RcodeRefused, nil
	}
	bound := d.limits.defaults()
	if len(prerequisites) > bound.updateRecords || len(updates) > bound.updateRecords-len(prerequisites) {
		return false, dns.RcodeRefused, nil
	}
	if err := d.ensureStore(); err != nil {
		return false, dns.RcodeServerFailure, err
	}
	if d.store != nil {
		d.store.mu.Lock()
		defer d.store.mu.Unlock()
		d.records = d.store.records
	}
	// RFC 2136 evaluates prerequisites against the current snapshot before
	// checking permissions and prescanning the Update section. Keeping this
	// order matters when a request contains both a failed prerequisite and an
	// invalid update record.
	if rcode := d.checkPrerequisites(prerequisites); rcode != dns.RcodeSuccess {
		return false, rcode, nil
	}
	if rcode := d.authorize(key, updates); rcode != dns.RcodeSuccess {
		return false, rcode, nil
	}
	if rcode := d.validateUpdates(updates); rcode != dns.RcodeSuccess {
		return false, rcode, nil
	}

	candidate, changed, explicitSOA := d.apply(updates)
	if !changed {
		return false, dns.RcodeSuccess, nil
	}
	if err := d.limits.check(candidate); err != nil {
		return false, dns.RcodeRefused, err
	}
	if !explicitSOA {
		bumpSerial(candidate)
	}
	view, err := d.build(candidate)
	if err != nil {
		return false, dns.RcodeServerFailure, fmt.Errorf("building candidate zone: %w", err)
	}
	if d.store != nil {
		if err := d.store.commit(candidate, view); err != nil {
			return false, dns.RcodeServerFailure, fmt.Errorf("committing zone: %w", err)
		}
	}
	d.install(candidate, view)
	return true, dns.RcodeSuccess, nil
}

func (d *DynUpdate) checkPrerequisites(prerequisites []dns.RR) int {
	valueDependent := make(map[rrsetKey][]dns.RR)
	for _, rr := range prerequisites {
		if rr == nil {
			return dns.RcodeFormatError
		}
		h := rr.Header()
		if h.Ttl != 0 {
			return dns.RcodeFormatError
		}
		if !inZone(d.Zone, h.Name) {
			return dns.RcodeNotZone
		}
		if unsupportedRRType(h.Rrtype) {
			return dns.RcodeNotImplemented
		}

		switch h.Class {
		case dns.ClassANY:
			if h.Rdlength != 0 || !validPrerequisiteType(h.Rrtype, true) {
				return dns.RcodeFormatError
			}
			if h.Rrtype == dns.TypeANY {
				if !d.nameInUse(h.Name) {
					return dns.RcodeNameError
				}
			} else if !d.rrsetExists(h.Name, h.Rrtype) {
				return dns.RcodeNXRrset
			}

		case dns.ClassNONE:
			if h.Rdlength != 0 || !validPrerequisiteType(h.Rrtype, true) {
				return dns.RcodeFormatError
			}
			if h.Rrtype == dns.TypeANY {
				if d.nameInUse(h.Name) {
					return dns.RcodeYXDomain
				}
			} else if d.rrsetExists(h.Name, h.Rrtype) {
				return dns.RcodeYXRrset
			}

		case dns.ClassINET:
			if !validPrerequisiteType(h.Rrtype, false) {
				return dns.RcodeFormatError
			}
			key := rrsetKey{name: canonicalName(h.Name), rrType: h.Rrtype}
			valueDependent[key] = append(valueDependent[key], rr)

		default:
			return dns.RcodeFormatError
		}
	}

	for key, want := range valueDependent {
		if !sameRRset(d.rrset(key.name, key.rrType), want) {
			return dns.RcodeNXRrset
		}
	}
	return dns.RcodeSuccess
}

func validPrerequisiteType(rrType uint16, allowAny bool) bool {
	if !knownRRType(rrType) || rrType == dns.TypeNone {
		return false
	}
	if rrType == dns.TypeANY {
		return allowAny
	}
	return !isQueryMetaType(rrType)
}

type rrsetKey struct {
	name   string
	rrType uint16
}

func (d *DynUpdate) authorize(key string, updates []dns.RR) int {
	for _, rr := range updates {
		if rr == nil {
			return dns.RcodeFormatError
		}
		if !d.allows(key, rr.Header().Name, rr.Header().Rrtype) {
			return dns.RcodeRefused
		}
	}
	return dns.RcodeSuccess
}

func (d *DynUpdate) validateUpdates(updates []dns.RR) int {
	for _, rr := range updates {
		if rr == nil {
			return dns.RcodeFormatError
		}
		h := rr.Header()
		if !inZone(d.Zone, h.Name) {
			return dns.RcodeNotZone
		}
		if !knownRRType(h.Rrtype) || h.Rrtype == dns.TypeNone {
			return dns.RcodeFormatError
		}

		switch h.Class {
		case dns.ClassINET:
			if isQueryMetaType(h.Rrtype) {
				return dns.RcodeFormatError
			}
			if h.Rrtype == dns.TypeSOA {
				if canonicalName(h.Name) != d.Zone {
					return dns.RcodeFormatError
				}
				soa, ok := rr.(*dns.SOA)
				// RFC 2136 sections 4.2 and 7.11 prohibit zero for
				// interoperability with older DNS implementations.
				if !ok || soa.Serial == 0 {
					return dns.RcodeFormatError
				}
			}
		case dns.ClassANY:
			if h.Ttl != 0 || h.Rdlength != 0 || isQueryMetaType(h.Rrtype) && h.Rrtype != dns.TypeANY {
				return dns.RcodeFormatError
			}
		case dns.ClassNONE:
			if h.Ttl != 0 || h.Rrtype == dns.TypeANY || isQueryMetaType(h.Rrtype) {
				return dns.RcodeFormatError
			}
		default:
			return dns.RcodeFormatError
		}

		if unsupportedRRType(h.Rrtype) {
			return dns.RcodeNotImplemented
		}
	}

	return dns.RcodeSuccess
}

// unsupportedRRType identifies records whose contents become invalid when a
// different RRset is updated without regenerating its associated metadata.
// This stage deliberately fails closed instead of serving stale DNSSEC or
// zone-digest data. The list includes obsolete DNSSEC types because they are
// still representable by miekg/dns and can otherwise enter a seed zone.
func unsupportedRRType(rrType uint16) bool {
	switch rrType {
	case dns.TypeSIG, dns.TypeKEY, dns.TypeNXT,
		dns.TypeDS, dns.TypeRRSIG, dns.TypeNSEC, dns.TypeDNSKEY,
		dns.TypeNSEC3, dns.TypeNSEC3PARAM,
		dns.TypeTALINK, dns.TypeCDS, dns.TypeCDNSKEY,
		dns.TypeZONEMD, dns.TypeTA, dns.TypeDLV:
		return true
	default:
		return false
	}
}

// apply follows RFC 2136 section 3.4.2 against a private copy. The boolean
// explicitSOA reports whether an accepted SOA update supplied the new serial;
// otherwise the server increments the serial after any real change.
func (d *DynUpdate) apply(updates []dns.RR) ([]dns.RR, bool, bool) {
	records := cloneRecords(d.records)
	changed := false
	explicitSOA := false

	for _, rr := range updates {
		h := rr.Header()
		name := canonicalName(h.Name)
		apex := name == d.Zone

		switch h.Class {
		case dns.ClassINET:
			switch h.Rrtype {
			case dns.TypeSOA:
				current := soaAt(records, d.Zone)
				incoming, ok := rr.(*dns.SOA)
				if !ok || current == nil || !serialGreater(incoming.Serial, current.Serial) {
					continue
				}
				records, _ = removeRecords(records, func(existing dns.RR) bool {
					return canonicalName(existing.Header().Name) == name && existing.Header().Rrtype == dns.TypeSOA
				})
				records = append(records, dns.Copy(rr))
				changed = true
				explicitSOA = true

			case dns.TypeCNAME:
				if hasOtherData(records, name) {
					continue
				}
				existing := rrsetOf(records, name, dns.TypeCNAME)
				if len(existing) == 1 && sameRR(existing[0], rr) && existing[0].Header().Ttl == h.Ttl {
					continue
				}
				records, _ = removeRecords(records, func(existing dns.RR) bool {
					return canonicalName(existing.Header().Name) == name && existing.Header().Rrtype == dns.TypeCNAME
				})
				records = append(records, dns.Copy(rr))
				changed = true

			default:
				if hasCNAME(records, name) && !cnameCompatibleType(h.Rrtype) {
					continue
				}
				if index := findRR(records, rr); index >= 0 {
					if records[index].Header().Ttl != h.Ttl {
						records[index] = dns.Copy(rr)
						changed = true
					}
					continue
				}
				records = append(records, dns.Copy(rr))
				changed = true
			}

		case dns.ClassANY:
			if h.Rrtype == dns.TypeANY {
				var removed bool
				records, removed = removeRecords(records, func(existing dns.RR) bool {
					if canonicalName(existing.Header().Name) != name {
						return false
					}
					if apex && (existing.Header().Rrtype == dns.TypeSOA || existing.Header().Rrtype == dns.TypeNS) {
						return false
					}
					return true
				})
				changed = changed || removed
				continue
			}
			if apex && (h.Rrtype == dns.TypeSOA || h.Rrtype == dns.TypeNS) {
				continue
			}
			var removed bool
			records, removed = removeRecords(records, func(existing dns.RR) bool {
				return canonicalName(existing.Header().Name) == name && existing.Header().Rrtype == h.Rrtype
			})
			changed = changed || removed

		case dns.ClassNONE:
			if apex && h.Rrtype == dns.TypeSOA {
				continue
			}
			if apex && h.Rrtype == dns.TypeNS && countRRset(records, name, dns.TypeNS) <= 1 {
				continue
			}
			if index := findRR(records, rr); index >= 0 {
				records = append(records[:index], records[index+1:]...)
				changed = true
			}
		}
	}

	return records, changed, explicitSOA
}

func (d *DynUpdate) reply(w dns.ResponseWriter, r *dns.Msg, rcode int) (int, error) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Opcode = dns.OpcodeUpdate
	m.Rcode = rcode
	m.Authoritative = true
	if err := w.WriteMsg(m); err != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil
}
