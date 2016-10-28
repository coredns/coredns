package middleware

import (
	"net"

	"github.com/miekg/coredns/middleware/etcd/msg"
	"github.com/miekg/coredns/middleware/pkg/dnsutil"
	"github.com/miekg/coredns/request"

	"github.com/miekg/dns"
)

// Backend defines a (dynamic) backend that returns a slice of service definitions.
type Backend interface {
	// Services communitates with the backend to retrieve the service defintion. Exact indicates
	// on exact much are that we are allowed to recurs.
	Services(state request.Request, exact bool, opt Options) ([]msg.Service, []msg.Service, error)

	// Lookup is used to 
			m1, e1 := e.Proxy.Lookup(state, target, state.QType())

	// Etcd thing Note sure we want this...
	PathPrefix

	// IsNameError return true if err indicated a record not found condition
	IsNameError(err error) bool
}

// Options are extra options that can be specified for a lookup.
type Options struct {
	Debug string // This is a debug query. A query prefixed with debug.o-o
}

// A returns A records from backend or an error.
func A(b Backend, zone string, state request.Request, previousRecords []dns.RR, opt Options) (records []dns.RR, debug []msg.Service, err error) {
	services, debug, err := b.Services(state, false, opt)
	if err != nil {
		return nil, debug, err
	}

	for _, serv := range services {
		ip := net.ParseIP(serv.Host)
		switch {
		case ip == nil:
			if Name(state.Name()).Matches(dns.Fqdn(serv.Host)) {
				// x CNAME x is a direct loop, don't add those
				continue
			}

			newRecord := serv.NewCNAME(state.QName(), serv.Host)
			if len(previousRecords) > 7 {
				// don't add it, and just continue
				continue
			}
			if dnsutil.DuplicateCNAME(newRecord, previousRecords) {
				continue
			}

			state1 := state.NewWithQuestion(serv.Host, state.QType())
			nextRecords, nextDebug, err := A(b, zone, state1, append(previousRecords, newRecord), opt)

			if err == nil {
				// Not only have we found something we should add the CNAME and the IP addresses.
				if len(nextRecords) > 0 {
					records = append(records, newRecord)
					records = append(records, nextRecords...)
					debug = append(debug, nextDebug...)
				}
				continue
			}
			// This means we can not complete the CNAME, try to look else where.
			target := newRecord.Target
			if dns.IsSubDomain(zone, target) {
				// We should already have found it
				continue
			}
			// Lookup
			m1, e1 := e.Proxy.Lookup(state, target, state.QType())
			if e1 != nil {
				debugMsg := msg.Service{Key: msg.Path(target, e.PathPrefix), Host: target, Text: " IN " + state.Type() + ": " + e1.Error()}
				debug = append(debug, debugMsg)
				continue
			}
			// Len(m1.Answer) > 0 here is well?
			records = append(records, newRecord)
			records = append(records, m1.Answer...)
			continue
		case ip.To4() != nil:
			records = append(records, serv.NewA(state.QName(), ip.To4()))
		case ip.To4() == nil:
			// nodata?
		}
	}
	return records, debug, nil
}
