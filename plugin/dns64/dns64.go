// Package dns64 implements a plugin that performs DNS64.
//
// See: RFC 6147 (https://tools.ietf.org/html/rfc6147)
package dns64

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

// DNS64 performs DNS64.
type DNS64 struct {
	Next         plugin.Handler
	Prefix       *net.IPNet
	TranslateAll bool // Not comply with 5.1.1
	AllowIPv4    bool
	Upstream     UpstreamInt
}

// ServeDNS implements the plugin.Handler interface.
func (d *DNS64) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	// Don't proxy if we don't need to.
	if !d.requestShouldIntercept(&request.Request{W: w, Req: r}) {
		return d.Next.ServeDNS(ctx, w, r)
	}

	// Pass the request to the next plugin in the chain, but intercept the response.
	nw := nonwriter.New(w)
	origRc, origErr := d.Next.ServeDNS(ctx, nw, r)
	if nw.Msg == nil { // somehow we didn't get a response (or raw bytes were written)
		return origRc, origErr
	}

	// If the response doesn't need DNS64, short-circuit.
	if !d.responseShouldDNS64(nw.Msg) {
		w.WriteMsg(nw.Msg)
		return origRc, origErr
	}

	// otherwise do the actual DNS64 request and response synthesis
	msg, err := d.DoDNS64(ctx, w, r, nw.Msg)
	if err != nil {
		// err means we weren't able to even issue the A request
		// to CoreDNS upstream
		return dns.RcodeServerFailure, err
	}

	RequestsTranslatedCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
	w.WriteMsg(msg)
	return msg.MsgHdr.Rcode, nil
}

// Name implements the Handler interface.
func (d *DNS64) Name() string { return "dns64" }

// requestShouldIntercept returns true if the request represents one that is eligible
// for DNS64 rewriting:
// 1. The request came in over IPv6 or the 'allow_ipv4' option is set
// 2. The request is of type AAAA
// 3. The request is of class INET
func (d *DNS64) requestShouldIntercept(req *request.Request) bool {
	// Make sure that request came in over IPv4 unless AllowIPv4 option is enabled.
	// Translating requests without taking into consideration client (source) IP might be problematic in dual-stack networks.
	if !d.AllowIPv4 && req.Family() == 1 {
		return false
	}

	// Do not modify if question is not AAAA or not of class IN. See RFC 6147 5.1
	return req.QType() == dns.TypeAAAA && req.QClass() == dns.ClassINET
}

// responseShouldDNS64 returns true if the response indicates we should attempt
// DNS64 rewriting:
// 1. The response has no valid (RFC 5.1.4) AAAA records (RFC 5.1.1)
// 2. The response code (RCODE) is not 3 (Name Error) (RFC 5.1.2)
//
// Note that requestShouldIntercept must also have been true, so the request
// is known to be of type AAAA.
func (d *DNS64) responseShouldDNS64(origResponse *dns.Msg) bool {
	ty, _ := response.Typify(origResponse, time.Now().UTC())

	// Handle NameError normally. See RFC 6147 5.1.2
	// All other error types are "equivalent" to empty response
	if ty == response.NameError {
		return false
	}

	// If we've configured to always translate, well, then always translate.
	if d.TranslateAll {
		return true
	}

	// if response includes AAAA record, no need to rewrite
	for _, rr := range origResponse.Answer {
		if rr.Header().Rrtype == dns.TypeAAAA {
			return false
		}
	}
	return true
}

// DoDNS64 takes an (empty) response to an AAAA question, issues the A request,
// and synthesizes the answer. Returns the response message, or error on internal failure.
func (d *DNS64) DoDNS64(ctx context.Context, w dns.ResponseWriter, r *dns.Msg, origResponse *dns.Msg) (*dns.Msg, error) {
	req := request.Request{W: w, Req: r} // req is unused
	resp, err := d.Upstream.Lookup(ctx, req, req.Name(), dns.TypeA)
	if err != nil {
		return nil, err
	}
	out := d.Synthesize(r, origResponse, resp)
	return out, nil
}

// Synthesize merges the AAAA response and the records from the A response
func (d *DNS64) Synthesize(origReq, origResponse, resp *dns.Msg) *dns.Msg {
	ret := dns.Msg{}
	ret.SetReply(origReq)

	// persist truncated state of AAAA response
	ret.Truncated = resp.Truncated

	// 5.3.2: DNS64 MUST pass the additional section unchanged
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
	for _, rr := range resp.Answer {
		header := rr.Header()
		// 5.3.3: All other RR's MUST be returned unchanged
		if header.Rrtype != dns.TypeA {
			ret.Answer = append(ret.Answer, rr)
			continue
		}

		aaaa, _ := to6(d.Prefix, rr.(*dns.A).A)

		// ttl is min of SOA TTL and A TTL
		ttl := SOATtl
		if rr.Header().Ttl < ttl {
			ttl = rr.Header().Ttl
		}

		// Replace A answer with a DNS64 AAAA answer
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
	return &ret
}

// to6 takes a prefix and IPv4 address and returns an IPv6 address according to RFC 6052.
func to6(prefix *net.IPNet, addr net.IP) (net.IP, error) {
	addr = addr.To4()
	if addr == nil {
		return nil, errors.New("not a valid IPv4 address")
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
