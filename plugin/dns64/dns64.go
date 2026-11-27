// Package dns64 implements a plugin that performs DNS64.
//
// See: RFC 6147 (https://www.rfc-editor.org/info/rfc6147/)
package dns64

import (
	"context"
	"errors"
	"fmt"
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

// synthesisCtxKey marks a context as belonging to dns64's own nested A lookup.
type synthesisCtxKey struct{}

func markSynthesisLookup(ctx context.Context) context.Context {
	return context.WithValue(ctx, synthesisCtxKey{}, true)
}

// IsSynthesisLookup reports whether ctx belongs to the nested A lookup dns64
// issues while answering an AAAA query. Other translation plugins (siit) use it
// to stay consistent with dns64.
func IsSynthesisLookup(ctx context.Context) bool {
	v, _ := ctx.Value(synthesisCtxKey{}).(bool)
	return v
}

type excludedCtxKey struct{}

// Excluded collects AAAA addresses that a plugin further down the chain (siit)
// removed from the response to an AAAA query. dns64 never synthesizes these
// addresses, even when it would derive them from a native A record.
type Excluded struct{ addrs []net.IP }

// Add records ip as excluded. It is a no-op on a nil receiver.
func (e *Excluded) Add(ip net.IP) {
	if e != nil {
		e.addrs = append(e.addrs, ip)
	}
}

func (e *Excluded) contains(ip net.IP) bool {
	if e == nil {
		return false
	}
	for _, a := range e.addrs {
		if a.Equal(ip) {
			return true
		}
	}
	return false
}

// ExcludedFromContext returns the collector dns64 installed for the AAAA query
// being served, or nil when dns64 is not handling this query.
func ExcludedFromContext(ctx context.Context) *Excluded {
	e, _ := ctx.Value(excludedCtxKey{}).(*Excluded)
	return e
}

// dropExcluded removes from msg the synthesized AAAA records that are in ex. If
// that leaves no AAAA, the response is a negative one and takes its authority
// section (the SOA) from the response to the original AAAA query, not from the
// positive A answer, whose NS set would read as a referral.
func dropExcluded(msg, orig *dns.Msg, ex *Excluded) {
	if len(ex.addrs) == 0 {
		return
	}
	kept := msg.Answer[:0]
	removed, aaaa := 0, 0
	for _, rr := range msg.Answer {
		if a, ok := rr.(*dns.AAAA); ok {
			if ex.contains(a.AAAA) {
				removed++
				continue
			}
			aaaa++
		}
		kept = append(kept, rr)
	}
	if removed == 0 {
		return
	}
	msg.Answer = kept
	msg.AuthenticatedData = false
	if aaaa == 0 {
		msg.Ns = orig.Ns
	}
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
		return plugin.NextOrFailure(d.Name(), d.Next, ctx, w, r)
	}

	// Plugins below us (siit) report here the AAAA addresses they removed.
	ex := &Excluded{}
	ctx = context.WithValue(ctx, excludedCtxKey{}, ex)

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

	dropExcluded(msg, nw.Msg, ex)

	requestsTranslatedCount.WithLabelValues(metrics.WithServer(ctx)).Inc()
	w.WriteMsg(msg)
	return dns.RcodeSuccess, nil
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
	resp, err := d.Upstream.Lookup(markSynthesisLookup(ctx), req, req.Name(), dns.TypeA)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("dns64: upstream returned no response")
	}
	out := d.Synthesize(r, origResponse, resp)
	return out, nil
}

// Synthesize merges the AAAA response and the records from the A response
func (d *DNS64) Synthesize(origReq, origResponse, resp *dns.Msg) *dns.Msg {
	ret := dns.Msg{}
	ret.SetReply(origReq)
	ret.Rcode = resp.Rcode

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
		ttl := min(rr.Header().Ttl, SOATtl)

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
