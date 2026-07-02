// Package siit implements a plugin that performs SIIT.
//
// See: RFC 6145 (https://tools.ietf.org/html/rfc6145)
// See: RFC 7757 (https://tools.ietf.org/html/rfc7757)
package siit

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// UpstreamInt wraps the Upstream API for dependency injection during testing
type UpstreamInt interface {
	Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error)
}

// SIIT performs SIIT.
type SIIT struct {
	Next       plugin.Handler
	IPv6Prefix *net.IPNet
	Eam6       map[string]net.IP
	Eam4       map[string]net.IP
	Upstream   UpstreamInt
}

// ServeDNS implements the plugin.Handler interface.
func (d *SIIT) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	// Don't proxy if we don't need to.
	if !d.requestShouldIntercept(&request.Request{W: w, Req: r}) {
		return plugin.NextOrFailure(d.Name(), d.Next, ctx, w, r)
	}

	// Pass the request to the next plugin in the chain, but intercept the response.
	nw := nonwriter.New(w)
	origRc, origErr := d.Next.ServeDNS(ctx, nw, r)
	if nw.Msg == nil { // somehow we didn't get a response (or raw bytes were written)
		return origRc, origErr
	}

	// If the response doesn't need SIIT, short-circuit.
	if !d.responseShouldSIIT(&request.Request{W: w, Req: r}, nw.Msg) {
		w.WriteMsg(nw.Msg)
		return origRc, origErr
	}

	// otherwise do the actual SIIT request and response synthesis
	msg, err := d.DoSIIT(ctx, w, r, nw.Msg)
	if err != nil {
		// err means we weren't able to even issue the A or AAAA request
		// to CoreDNS upstream
		return dns.RcodeServerFailure, err
	}

	RequestsTranslatedCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
	w.WriteMsg(msg)
	return msg.Rcode, nil
}

// Name implements the Handler interface.
func (d *SIIT) Name() string { return "siit" }

// requestShouldIntercept returns true if the request represents one that is eligible
// for SIIT rewriting:
// 2. The request is of type AAAA or A
// 3. The request is of class INET
func (d *SIIT) requestShouldIntercept(req *request.Request) bool {
	// Do not modify if question is not AAAA or A or not of class IN. See RFC 6147 5.1
	return (req.QType() == dns.TypeA || req.QType() == dns.TypeAAAA) && req.QClass() == dns.ClassINET
}

// responseShouldDNS64 returns true if the response indicates we should attempt
// SIIT rewriting:
// 1. The response has no valid (RFC 5.1.4) AAAA records (RFC 5.1.1) or A records (depending on the source)
// 2. The response code (RCODE) is not 3 (Name Error) (RFC 5.1.2)
//
// Note that requestShouldIntercept must also have been true, so the request
// is known to be of type AAAA or A.
func (d *SIIT) responseShouldSIIT(req *request.Request, origResponse *dns.Msg) bool {
	ty, _ := response.Typify(origResponse, time.Now().UTC())

	// Handle NameError normally. See RFC 6147 5.1.2
	// All other error types are "equivalent" to empty response
	if ty == response.NameError {
		return false
	}

	// if response includes AAAA record for an AAAA request, no need to rewrite
	// same for A record and A request
	for _, rr := range origResponse.Answer {
		if rr.Header().Rrtype == dns.TypeAAAA && req.QType() == dns.TypeAAAA {
			return false
		}
		if rr.Header().Rrtype == dns.TypeA && req.QType() == dns.TypeA {
			return false
		}
	}
	return true
}

// DoSIIT takes an (empty) response to an AAAA question, issues the A request,
// and synthesizes the answer. Returns the response message, or error on internal failure.
// It also do the A question for the AAAA request.
func (d *SIIT) DoSIIT(ctx context.Context, w dns.ResponseWriter, r *dns.Msg, origResponse *dns.Msg) (*dns.Msg, error) {
	req := request.Request{W: w, Req: r}
	defaultreq := dns.TypeA

	if req.QType() == dns.TypeA {
		defaultreq = dns.TypeAAAA
	}

	resp, err := d.Upstream.Lookup(ctx, req, req.Name(), defaultreq)

	if err != nil {
		return nil, err
	}
	out := d.Synthesize(r, origResponse, resp)
	return out, nil
}

// Synthesize merges the AAAA response and the records from the A response
func (d *SIIT) Synthesize(origReq, origResponse, resp *dns.Msg) *dns.Msg {
	ret := dns.Msg{}
	ret.SetReply(origReq)

	// persist truncated state of AAAA or A response
	ret.Truncated = resp.Truncated

	// 5.3.2: SIIT MUST pass the additional section unchanged
	ret.Extra = resp.Extra
	ret.Ns = resp.Ns

	// 5.1.7: The TTL is the minimum of the A RR and the SOA RR. If SOA is
	// unknown, then the TTL is the minimum of A TTL and 600
	SOATtl := uint32(600) // Default NS record TTL
	for _, ns := range origResponse.Ns {
		if ns.Header().Rrtype == dns.TypeSOA {
			SOATtl = ns.Header().Ttl
		}
	}

	ret.Answer = make([]dns.RR, 0, len(resp.Answer))
	// convert A records to AAAA records
	// and vice-versa
	for _, rr := range resp.Answer {
		header := rr.Header()
		// 5.3.3: All other RR's MUST be returned unchanged
		if header.Rrtype != dns.TypeA && header.Rrtype != dns.TypeAAAA {
			ret.Answer = append(ret.Answer, rr)
			continue
		}

		if header.Rrtype == dns.TypeA {
			aaaa, _ := to6(d.IPv6Prefix, d.Eam6, rr.(*dns.A).A)

			// ttl is min of SOA TTL and A TTL
			ttl := min(rr.Header().Ttl, SOATtl)

			// Replace A answer with a SIIT AAAA answer
			ret.Answer = append(ret.Answer, &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   header.Name,
					Rrtype: dns.TypeAAAA,
					Class:  header.Class,
					Ttl:    ttl,
				},
				AAAA: aaaa,
			})
		}

		if header.Rrtype == dns.TypeAAAA {
			a, _ := to4(d.Eam4, rr.(*dns.AAAA).AAAA)

			// ttl is min of SOA TTL and A TTL
			ttl := min(rr.Header().Ttl, SOATtl)

			// Replace AAAA answer with a SIIT A answer
			ret.Answer = append(ret.Answer, &dns.A{
				Hdr: dns.RR_Header{
					Name:   header.Name,
					Rrtype: dns.TypeA,
					Class:  header.Class,
					Ttl:    ttl,
				},
				A: a,
			})
		}
	}
	return &ret
}

// to6 takes a prefix and IPv4 address or an eam and returns an IPv6 address.
func to6(prefix *net.IPNet, eam map[string]net.IP, addr net.IP) (net.IP, error) {
	addr = addr.To4()
	if addr == nil {
		return nil, errors.New("not a valid IPv4 address")
	}

	if eam[addr.String()] != nil {
		v6 := eam[addr.String()]
		return v6, nil
	}

	n, _ := prefix.Mask.Size()
	// Assumes prefix has been validated during setup
	v6 := make([]byte, 16)
	i, j := 0, 0

	for ; i < n/8; i++ {
		v6[i] = prefix.IP[i]
	}
	for ; i < 8; i, j = i+1, j+1 {
		v6[i] = addr[j]
	}
	if i == 8 {
		i++
	}
	for ; j < 4; i, j = i+1, j+1 {
		v6[i] = addr[j]
	}

	return v6, nil
}

// to4 takes an IPv6 address and an eam and returns an IPv4 address.
func to4(eam map[string]net.IP, addr net.IP) (net.IP, error) {
	addr = addr.To16()
	if addr == nil || addr.To4() != nil {
		return nil, errors.New("not a valid IPv6 address")
	}

	if eam[addr.String()] != nil {
		v4 := eam[addr.String()]
		return v4, nil
	}

	return nil, nil
}
