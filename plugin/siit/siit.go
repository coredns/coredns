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
// 2. The request is of type A
// 3. The request is of class INET
func (d *SIIT) requestShouldIntercept(req *request.Request) bool {
	// Do not modify if question is not A or not of class IN. See RFC 6147 5.1
	return (req.QType() == dns.TypeA) && req.QClass() == dns.ClassINET
}

// responseShouldSIIT returns true if the response indicates we should attempt
// SIIT rewriting:
// 1. The response has no valid (RFC 5.1.4) A records (RFC 5.1.1)
// 2. The response code (RCODE) is not 3 (Name Error) (RFC 5.1.2)
//
// Note that requestShouldIntercept must also have been true, so the request
// is known to be of type A.
func (d *SIIT) responseShouldSIIT(req *request.Request, origResponse *dns.Msg) bool {
	ty, _ := response.Typify(origResponse, time.Now().UTC())

	// Handle NameError normally. See RFC 6147 5.1.2
	// All other error types are "equivalent" to empty response
	if ty == response.NameError {
		return false
	}

	// if response includes A record for an A request, no need to rewrite
	for _, rr := range origResponse.Answer {
		if rr.Header().Rrtype == dns.TypeA && req.QType() == dns.TypeA {
			return false
		}
	}
	return true
}

// DoSIIT takes an (empty) response to an A question, issues the AAAA request,
// and synthesizes the answer. Returns the response message, or error on internal failure.
func (d *SIIT) DoSIIT(ctx context.Context, w dns.ResponseWriter, r *dns.Msg, origResponse *dns.Msg) (*dns.Msg, error) {
	req := request.Request{W: w, Req: r}
	defaultreq := dns.TypeAAAA

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
		if header.Rrtype != dns.TypeAAAA {
			ret.Answer = append(ret.Answer, rr)
			continue
		}

		if header.Rrtype == dns.TypeAAAA {
			a, _ := to4(d.Eam4, d.IPv6Prefix, rr.(*dns.AAAA).AAAA)

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
				A: a.To16(),
			})
		}
	}
	return &ret
}

// extractIPv4 reverses CoreDNS's dns64 embedding logic: given a v6 address
// that was built by embedding a v4 address into prefix, it extracts that
// v4 address back out. prefix must be a valid NAT64 prefix length per
// RFC 6052 (/32, /40, /48, /56, /64, or /96).
func extractIPv4(v6 net.IP, prefix *net.IPNet) net.IP {
	n, _ := prefix.Mask.Size()
	v6 = v6.To16()

	addr := make([]byte, 4)
	i, j := n/8, 0 // skip the prefix bytes, we don't need them back

	for ; i < 8; i, j = i+1, j+1 {
		addr[j] = v6[i]
	}
	if i == 8 {
		i++ // skip the reserved "u" byte
	}
	for ; j < 4; i, j = i+1, j+1 {
		addr[j] = v6[i]
	}

	return net.IP(addr)
}

// to4 takes an IPv6 address and an eam and returns an IPv4 address.
func to4(eam map[string]net.IP, ipv6prefix *net.IPNet, addr net.IP) (net.IP, error) {
	addr = addr.To16()
	if addr == nil || addr.To4() != nil {
		return nil, errors.New("not a valid IPv6 address")
	}

	if ipv6prefix.Contains(addr) {
		v4 := extractIPv4(addr, ipv6prefix)
		return v4, nil
	}

	if eam[addr.String()] != nil {
		v4 := eam[addr.String()]
		return v4, nil
	}

	return nil, nil
}
