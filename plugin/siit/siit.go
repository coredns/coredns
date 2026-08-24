// Package siit implements a plugin that performs AAAA to A translation.
//
// See: RFC 6052 (https://tools.ietf.org/html/rfc6052)
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

// synthesisCtxKey marks a context as belonging to a translation plugin's own
// internal synthesis lookup (as opposed to a query that arrived from a
// client). Unexported so it can't collide with keys set by other packages.
type synthesisCtxKey struct{}

// markSynthesisLookup flags ctx as carrying a nested lookup issued by SIIT's
// own synthesis logic. The flag propagates through every
// plugin.Handler.ServeDNS(ctx, ...) call in the chain, including through
// other translation plugins such as dns64, whose own internal
// Upstream.Lookup calls reuse the context they were invoked with.
func markSynthesisLookup(ctx context.Context) context.Context {
	return context.WithValue(ctx, synthesisCtxKey{}, true)
}

// isSynthesisLookup reports whether ctx was produced by markSynthesisLookup.
func isSynthesisLookup(ctx context.Context) bool {
	v, _ := ctx.Value(synthesisCtxKey{}).(bool)
	return v
}

// ServeDNS implements the plugin.Handler interface.
func (d *SIIT) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	// A nested lookup issued as part of an in-flight synthesis (SIIT's own,
	// or another translation plugin's further up the chain, e.g. dns64)
	// must not be intercepted again — just pass it through. Otherwise SIIT
	// and dns64 chained together can trigger each other's internal lookups
	// indefinitely: SIIT's AAAA synthesis lookup re-enters the server and
	// reaches dns64, whose own internal A lookup (issued with that same
	// context) re-enters the server and reaches SIIT again as a plain A
	// query, which SIIT would otherwise translate by issuing yet another
	// AAAA lookup, and so on with no bound.
	if isSynthesisLookup(ctx) {
		return plugin.NextOrFailure(d.Name(), d.Next, ctx, w, r)
	}

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
	msg, synthesized, err := d.DoSIIT(ctx, w, r, nw.Msg)
	if err != nil {
		// err means we weren't able to even issue the A or AAAA request
		// to CoreDNS upstream
		return dns.RcodeServerFailure, err
	}

	if synthesized {
		RequestsTranslatedCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
	}
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
	if origResponse.Truncated {
		return false
	}

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
func (d *SIIT) DoSIIT(ctx context.Context, w dns.ResponseWriter, r *dns.Msg, origResponse *dns.Msg) (*dns.Msg, bool, error) {
	req := request.Request{W: w, Req: r}
	defaultreq := dns.TypeAAAA

	resp, err := d.Upstream.Lookup(markSynthesisLookup(ctx), req, req.Name(), defaultreq)

	if err != nil {
		return nil, false, err
	}

	if resp == nil {
		// Upstream.Lookup isn't expected to return (nil, nil), but guard
		// against it explicitly rather than let ServeDNS write a nil
		// message and then dereference msg.Rcode.
		return nil, false, errors.New("siit: upstream lookup returned no response")
	}

	if resp.Rcode != dns.RcodeSuccess {
		// Not a transport error — we got an answer, it's just SERVFAIL/
		// REFUSED/NXDOMAIN/etc. resp still carries the *internal* AAAA
		// question we issued, not the client's original A question.
		// RFC 6147 §5.4 requires the assembled response to carry the
		// original initiator's question/ID/transaction framing, so we
		// can't return resp as-is — rebuild the envelope from the
		// original request and copy over resp's RCODE, flags, and
		// authority/additional sections.
		return d.copyErrorResponse(r, resp), false, nil
	}

	out, synthesized := d.Synthesize(r, origResponse, resp)
	return out, synthesized, nil
}

// copyErrorResponse builds a response addressed to the client's original
// request — correct ID, Question, and QR/Opcode framing via SetReply — while
// carrying over the secondary AAAA lookup's RCODE, AA/RA/TC flags, answer,
// authority, and additional sections. Used when the internal AAAA lookup
// itself failed (SERVFAIL, NXDOMAIN, REFUSED, etc.) so the client never sees
// the internal AAAA question in reply to its A query.
func (d *SIIT) copyErrorResponse(origReq, resp *dns.Msg) *dns.Msg {
	ret := new(dns.Msg)
	ret.SetReply(origReq) // sets Id, Question, Opcode, RD/CD from origReq

	ret.Rcode = resp.Rcode
	ret.Authoritative = resp.Authoritative
	ret.RecursionAvailable = resp.RecursionAvailable
	ret.Truncated = resp.Truncated

	ret.Answer = resp.Answer
	ret.Ns = resp.Ns
	ret.Extra = resp.Extra

	return ret
}

// Synthesize merges the AAAA response and the records from the A response.
// The bool return reports whether at least one AAAA record was actually
// translated into a synthetic A record. false means the caller got back
// origResponse unchanged — nothing in the AAAA answer was mappable (no EAM
// entry, and the address wasn't in ipv6_prefix) — and this must not be
// counted as a translated request.
func (d *SIIT) Synthesize(origReq, origResponse, resp *dns.Msg) (*dns.Msg, bool) {
	ret := dns.Msg{}
	ret.SetReply(origReq)

	mappedAny := false

	ret.Authoritative = resp.Authoritative
	ret.RecursionAvailable = resp.RecursionAvailable
	ret.AuthenticatedData = false

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
			a, mapped := to4(d.Eam4, d.IPv6Prefix, rr.(*dns.AAAA).AAAA)
			if !mapped {
				// No EAM entry and address isn't in ipv6_prefix — nothing to synthesize
				// for this record; skip it rather than emitting an empty-RDATA A record.
				continue
			}

			mappedAny = true

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

	if !mappedAny {
		return origResponse, false
	}

	return &ret, true
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
func to4(eam map[string]net.IP, ipv6prefix *net.IPNet, addr net.IP) (net.IP, bool) {
	addr = addr.To16()
	if addr == nil || addr.To4() != nil {
		return nil, false
	}

	// RFC 7757 §3.3.2: search the EAM table first.
	if eam[addr.String()] != nil {
		v4 := eam[addr.String()]
		return v4, true
	}

	// Fall back to RFC 6052 algorithmic translation only if no EAM matched.
	if ipv6prefix.Contains(addr) {
		// RFC 6052 §2.2: byte 8 (bits 64-71, the "u" octet) is reserved and
		// MUST be zero. A correctly-configured prefix guarantees this for
		// its own bits (validated at setup time), but for prefix lengths
		// shorter than /96 that byte belongs to the address, not the
		// prefix, and an upstream AAAA answer isn't required to zero it.
		// Reject rather than silently translating from a malformed address.
		if addr[8] != 0 {
			return nil, false
		}
		v4 := extractIPv4(addr, ipv6prefix)
		return v4, true
	}

	return nil, false
}
