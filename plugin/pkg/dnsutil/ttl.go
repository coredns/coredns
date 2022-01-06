package dnsutil

import (
	"time"

	"github.com/coredns/coredns/plugin/pkg/response"

	"github.com/miekg/dns"
)

// MinimalTTL scans the message returns the lowest TTL found taking into the response.Type of the message.
func MinimalTTL(m *dns.Msg, mt response.Type) time.Duration {
	if mt != response.NoError && mt != response.NameError && mt != response.NoData {
		return MinimalDefaultTTL
	}

	// No records or OPT is the only record, return a short ttl as a fail safe.
	if len(m.Answer)+len(m.Ns) == 0 &&
		(len(m.Extra) == 0 || (len(m.Extra) == 1 && m.Extra[0].Header().Rrtype == dns.TypeOPT)) {
		return MinimalDefaultTTL
	}

	// Covers Cases where there is NoError also answer type doesnt match question and no SOA
	answerMatch := false
	if len(m.Question) > 0 && len(m.Ns) == 0 && len(m.Answer) > 0 {
		if m.Question[0].Qtype == dns.TypeA || m.Question[0].Qtype == dns.TypeAAAA {
			for _, r := range m.Answer {
				if m.Question[0].Qtype == r.Header().Rrtype {
					answerMatch = true
				}
			}
			if !answerMatch {
				return MinimalDefaultTTL
			}
		}
	}

	minTTL := MaximumDefaulTTL
	for _, r := range m.Answer {
		if r.Header().Ttl < uint32(minTTL.Seconds()) {
			minTTL = time.Duration(r.Header().Ttl) * time.Second
		}
	}
	for _, r := range m.Ns {
		if r.Header().Ttl < uint32(minTTL.Seconds()) {
			minTTL = time.Duration(r.Header().Ttl) * time.Second
		}
	}

	for _, r := range m.Extra {
		if r.Header().Rrtype == dns.TypeOPT {
			// OPT records use TTL field for extended rcode and flags
			continue
		}
		if r.Header().Ttl < uint32(minTTL.Seconds()) {
			minTTL = time.Duration(r.Header().Ttl) * time.Second
		}
	}
	return minTTL
}

const (
	// MinimalDefaultTTL is the absolute lowest TTL we use in CoreDNS.
	MinimalDefaultTTL = 5 * time.Second
	// MaximumDefaulTTL is the maximum TTL was use on RRsets in CoreDNS.
	MaximumDefaulTTL = 1 * time.Hour
)
