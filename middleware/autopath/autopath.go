package autopath

/*
Autopath is a hack; it shortcuts the client's search path resolution by performing
these lookups on the server...

The server has a copy of the client's search path and on receiving a query it first
establish if the suffix matches any of the configured path elements. If no match
can be found the query will be forward up the middleware chain without interference.

If the query is deemed to fall in the search path the server will perform the
queries with each element of the search path appendded in sequence until a
non-NXDOMAIN answer has been found until a successfull, non NXDOMAIN has been found.
That reply will then be returned to the client - with some CNAME hackery to let the client
accept the reply.

If non of the searches results in a unusable answer we... NODATA response????


It is assume the search path ordering is identical between server and client, so when we find
a search path match we continue from that point in the list and not the beginning - assuming
those queries have already been done by the client.
*/

import (
	"errors"

	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

type AutoPath struct {
	Next   middleware.Handler
	search []string
	// options ndots:5
}

func (a AutoPath) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if state.QClass() != dns.ClassINET {
		return dns.RcodeServerFailure, middleware.Error(a.Name(), errors.New("can only deal with ClassINET"))
	}

	do := a.inSearchPath(state.Name())
	if do == -1 { // Does not fall in the search path, normal middleware chaining
		// TOD(miek): should actually have a fallthrough thing for this; should be yes.
		return middleware.NextOrFailure(a.Name(), a.Next, ctx, w, r)
	}

	// Establish base name of the query. I.e what was originally asked.
	base, err := dnsutil.TrimZone(state.QName(), a.search[do]) // TOD(miek): we loose the original case of the query here.
	if err != nil {
		return dns.RcodeServerFailure, err
	}

	nw := NewNonWriter(w)
	for i := do; i < len(a.search); i++ {
		println("base", base, a.search[i])

		newQName := base + "." + a.search[i]
		r.Question[0].Name = newQName

		rcode, err := middleware.NextOrFailure(a.Name(), a.Next, ctx, nw, r)
		errt := ""
		if err != nil {
			errt = err.Error()
		}
		println("rcode", rcode, "err", errt)

	}
	return dns.RcodeServerFailure, nil
}

// inSearchPath return the index of the searchpath element that is the closest parent
// of state.Name(). It returns -1 when there is no match.
func (a AutoPath) inSearchPath(name string) int {
	z := middleware.Zones(a.search)
	sp := z.Matches(name)
	if sp == "" {
		return -1
	}
	// We could not use middleware.Zones in the above loop and do it in 1 go, but
	// a) middleware.Zones exists and b) this list of short.
	for i, s := range a.search {
		if s == sp {
			return i
		}
	}
	return -1
}

func (a AutoPath) Name() string { return "autopath" }
