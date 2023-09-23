// Package oneaddr is a plugin for rewriting responses to retain only single address
package oneaddr

import (
	"strings"

	"github.com/miekg/dns"
)

type dedupKey struct {
	name string
	rrtype uint16
	class uint16
}

type dedupSet = map[dedupKey]struct{}

func makeDedupKey(rec dns.RR) dedupKey {
	hdr := rec.Header()
	return dedupKey{
		name: strings.ToLower(hdr.Name),
		rrtype: hdr.Rrtype,
		class: hdr.Class,
	}
}

// OneAddrResponseWriter is a response writer that shuffles A, AAAA and MX records.
type OneAddrResponseWriter struct {
	dns.ResponseWriter
}

// WriteMsg implements the dns.ResponseWriter interface.
func (r *OneAddrResponseWriter) WriteMsg(res *dns.Msg) error {
	if res.Rcode != dns.RcodeSuccess {
		return r.ResponseWriter.WriteMsg(res)
	}

	if res.Question[0].Qtype == dns.TypeAXFR || res.Question[0].Qtype == dns.TypeIXFR {
		return r.ResponseWriter.WriteMsg(res)
	}

	if res.Question[0].Qtype != dns.TypeA && res.Question[0].Qtype != dns.TypeAAAA {
		return r.ResponseWriter.WriteMsg(res)
	}

	res.Answer = trimRecords(res.Answer)
	res.Extra = trimRecords(res.Extra)

	return r.ResponseWriter.WriteMsg(res)
}

func trimRecords(in []dns.RR) []dns.RR {
	seen := make(dedupSet)
	out := []dns.RR{}
	for _, rec := range in {
		switch rec.Header().Rrtype {
		case dns.TypeA, dns.TypeAAAA:
			key := makeDedupKey(rec)
			if _, ok := seen[key]; ok { continue }
			seen[key] = struct{}{}
			fallthrough
		default:
			out = append(out, rec)
		}
	}

	return out
}

// Write implements the dns.ResponseWriter interface.
func (r *OneAddrResponseWriter) Write(buf []byte) (int, error) {
	// Should we pack and unpack here to fiddle with the packet... Not likely.
	log.Warning("oneaddr plugin called with Write: not shuffling records")
	n, err := r.ResponseWriter.Write(buf)
	return n, err
}
