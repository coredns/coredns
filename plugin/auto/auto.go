// Package auto implements an on-the-fly loading file backend.
package auto

import (
	"context"
	"regexp"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type (
	// Auto holds the zones and the loader configuration for automatically loading zones.
	Auto struct {
		Next plugin.Handler
		*Zones

		metrics  *metrics.Metrics
		transfer *transfer.Transfer
		loader
	}

	loader struct {
		directory string
		template  string
		re        *regexp.Regexp

		ReloadInterval time.Duration
		upstream       *upstream.Upstream // Upstream for looking up names during the resolution process.
	}
)

// ServeDNS implements the plugin.Handler interface.
func (a Auto) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	qname := state.Name()

	// Precheck with the origins, i.e. are we allowed to look here?
	zone := plugin.Zones(a.Zones.Origins()).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(a.Name(), a.Next, ctx, w, r)
	}

	// Now the real zone.
	zone = plugin.Zones(a.Zones.Names()).Matches(qname)
	if zone == "" {
		// If no next plugin is configured, it's more correct to return REFUSED as auto acts as an authoritative server
		if a.Next == nil {
			return dns.RcodeRefused, nil
		}
		return plugin.NextOrFailure(a.Name(), a.Next, ctx, w, r)
	}

	a.RLock()
	z, ok := a.Z[zone]
	a.RUnlock()

	if !ok || z == nil {
		return dns.RcodeServerFailure, nil
	}

	// If transfer is not loaded, we'll see these, answer with refused (no transfer allowed).
	if state.QType() == dns.TypeAXFR || state.QType() == dns.TypeIXFR {
		return dns.RcodeRefused, nil
	}

	answer, ns, extra, result := z.Lookup(ctx, state, qname)

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Answer, m.Ns, m.Extra = answer, ns, extra

	switch result {
	case file.Success:
	case file.NoData:
	case file.NameError:
		m.Rcode = dns.RcodeNameError
	case file.Delegation:
		m.Authoritative = false
	case file.ServerFailure:
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
func (a Auto) Name() string { return "auto" }
