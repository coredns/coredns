// Package file implements a file backend.
package file

import (
	"context"
	"fmt"
	"io"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("file")

type (
	// File is the plugin that reads zone data from disk.
	File struct {
		Next plugin.Handler
		Zones
		transfer *transfer.Transfer

		Fall fall.F
	}

	// Zones maps zone names to a *Zone.
	Zones struct {
		Z     map[string]*Zone // A map mapping zone (origin) to the Zone's data
		Names []string         // All the keys from the map Z as a string slice.
	}
)

// ServeDNS implements the plugin.Handle interface.
func (f File) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	qname := state.Name()
	// TODO(miek): match the qname better in the map
	zone := plugin.Zones(f.Zones.Names).Matches(qname)
	if zone == "" {
		// If no next plugin is configured, it's more correct to return REFUSED as file acts as an authoritative server
		if f.Next == nil {
			return dns.RcodeRefused, nil
		}
		return plugin.NextOrFailure(f.Name(), f.Next, ctx, w, r)
	}

	z, ok := f.Z[zone]
	if !ok || z == nil {
		return dns.RcodeServerFailure, nil
	}

	// If transfer is not loaded, we'll see these, answer with refused (no transfer allowed).
	if state.QType() == dns.TypeAXFR || state.QType() == dns.TypeIXFR {
		return dns.RcodeRefused, nil
	}

	// This is only for when we are a secondary zones.
	if r.Opcode == dns.OpcodeNotify {
		if z.isNotify(state) {
			m := new(dns.Msg)
			m.SetReply(r)
			m.Authoritative = true
			w.WriteMsg(m)

			log.Infof("Notify from %s for %s: checking transfer", state.IP(), zone)
			ok, err := z.shouldTransfer()
			if ok {
				z.TransferIn()
			} else {
				log.Infof("Notify from %s for %s: no SOA serial increase seen", state.IP(), zone)
			}
			if err != nil {
				log.Warningf("Notify from %s for %s: failed primary check: %s", state.IP(), zone, err)
			}
			return dns.RcodeSuccess, nil
		}
		log.Infof("Dropping notify from %s for %s", state.IP(), zone)
		return dns.RcodeSuccess, nil
	}

	z.RLock()
	exp := z.Expired
	z.RUnlock()
	if exp {
		log.Errorf("Zone %s is expired", zone)
		return dns.RcodeServerFailure, nil
	}

	answer, ns, extra, result := z.Lookup(ctx, state, qname)

	// Only on NXDOMAIN we will fallthrough.
	// `z.Lookup` can also return NOERROR for NXDOMAIN see comment see comment "Hacky way to get around empty-non-terminals" inside `Zone.Lookup`.
	// It's safe to fallthrough with `result` Sucess (NOERROR) since all other return points in Lookup with Success have answer(s).
	if len(answer) == 0 && (result == NameError || result == Success) && f.Fall.Through(qname) {
		return plugin.NextOrFailure(f.Name(), f.Next, ctx, w, r)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Answer, m.Ns, m.Extra = answer, ns, extra

	switch result {
	case Success:
	case NoData:
	case NameError:
		m.Rcode = dns.RcodeNameError
	case Delegation:
		m.Authoritative = false
	case ServerFailure:
		// If the result is SERVFAIL and the answer is non-empty, then the SERVFAIL came from an
		// external CNAME lookup and the answer contains the CNAME with no target record. We should
		// write the CNAME record to the client instead of sending an empty SERVFAIL response.
		if len(m.Answer) == 0 {
			return dns.RcodeServerFailure, nil
		}
		//  The rcode in the response should be the rcode received from the target lookup. RFC 6604 section 3
		m.Rcode = dns.RcodeServerFailure
	}

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
	return fmt.Sprintf("%s for origin %s in file %s, with %d SOA serial", s.err, s.origin, s.zone, s.serial)
}

// Parse parses the zone in filename and returns a new Zone or an error.
// If serial >= 0 it will reload the zone, if the SOA hasn't changed
// it returns an error indicating nothing was read.
func Parse(f io.Reader, origin, fileName string, serial int64) (*Zone, error) {
	zp := dns.NewZoneParser(f, dns.Fqdn(origin), fileName)
	zp.SetIncludeAllowed(true)
	z := NewZone(origin, fileName)
	seenSOA := false
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		if !seenSOA {
			if s, ok := rr.(*dns.SOA); ok {
				seenSOA = true

				// -1 is valid serial is we failed to load the file on startup.

				if serial >= 0 && s.Serial == uint32(serial) { // same serial
					return nil, &serialErr{err: "no change in SOA serial", origin: origin, zone: fileName, serial: serial}
				}
			}
		}

		if err := z.Insert(rr); err != nil {
			return nil, err
		}
	}
	if !seenSOA {
		return nil, fmt.Errorf("file %q has no SOA record for origin %s", fileName, origin)
	}
	if zp.Err() != nil {
		return nil, fmt.Errorf("failed to parse file %q for origin %s with error %v", fileName, origin, zp.Err())
	}

	if err := zp.Err(); err != nil {
		return nil, err
	}

	return z, nil
}
