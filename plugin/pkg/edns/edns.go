// Package edns provides function useful for adding/inspecting OPT records to/in messages.
package edns

import (
	"errors"
	"sync"

	"github.com/miekg/dns"
)

var sup = &supported{m: make(map[uint16]struct{})}

type supported struct {
	m map[uint16]struct{}
	sync.RWMutex
}

// SetSupportedOption adds an EDNS0 option to the set of options that CoreDNS may copy from a request to an
// OPT-less response. Plugins typically call this in their setup code to signal support for a custom option.
func SetSupportedOption(option uint16) {
	sup.Lock()
	sup.m[option] = struct{}{}
	sup.Unlock()
}

// SupportedOption returns true if the option code was explicitly registered.
func SupportedOption(option uint16) bool {
	sup.RLock()
	_, ok := sup.m[option]
	sup.RUnlock()
	return ok
}

// Version checks the EDNS version in the request. If error
// is nil everything is OK and we can invoke the plugin. If non-nil, the
// returned Msg is valid to be returned to the client (and should).
func Version(req *dns.Msg) (*dns.Msg, error) {
	opt := req.IsEdns0()
	if opt == nil {
		return nil, nil
	}
	if opt.Version() == 0 {
		return nil, nil
	}
	m := new(dns.Msg)
	m.SetReply(req)

	o := new(dns.OPT)
	o.Hdr.Name = "."
	o.Hdr.Rrtype = dns.TypeOPT
	o.SetVersion(0)
	m.Rcode = dns.RcodeBadVers
	o.SetExtendedRcode(dns.RcodeBadVers)
	m.Extra = []dns.RR{o}

	return m, errors.New("EDNS0 BADVERS")
}

// Size returns a normalized size based on proto.
func Size(proto string, size uint16) uint16 {
	if proto == "tcp" {
		return dns.MaxMsgSize
	}
	if size < dns.MinMsgSize {
		return dns.MinMsgSize
	}
	return size
}
