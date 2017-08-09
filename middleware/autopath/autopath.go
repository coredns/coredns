package autopath

/*
Autopath is a hack; it shortcuts the client's search path resolution by performing
these lookups on the server...

The server has a copy (via AutoPathFunc) of the client's search path and on
receiving a query it first establish if the suffix matches the FIRST configured
element. If no match can be found the query will be forwarded up the middleware
chain without interference (iff 'fallthrough' has been set).

If the query is deemed to fall in the search path the server will perform the
queries with each element of the search path appended in sequence until a
non-NXDOMAIN answer has been found. That reply will then be returned to the
client - with some CNAME hackery to let the client accept the reply.

If all queries return NXDOMAIN we return the original as-is and let the client
continue searching. The client will go to the next element in the search path,
but we won’t do any more autopathing. It means that in the failure case, you do
more work, since the server looks it up, then the client still needs to go
through the search path.

It is assume the search path ordering is identical between server and client.
*/

import (
	"errors"
	"fmt"

	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

// AutoPathFunc defines the function middleware should implement to return a search
// path to the autopath middleware. The last element of the slice must be the empty string.
// If AutoPathFunc returns a non-nil error no autopathing is performed.
type AutoPathFunc func(request.Request) ([]string, error)

// Autopath perform autopath: service side search path completion.
type AutoPath struct {
	Next  middleware.Handler
	Zones []string

	// Search always includes "" as the last element, so we try the base query with out any search paths added as well.
	search     []string
	searchFunc AutoPathFunc
}

func (a *AutoPath) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	if state.QClass() != dns.ClassINET {
		return dns.RcodeServerFailure, middleware.Error(a.Name(), errors.New("can only deal with ClassINET"))
	}

	var err error
	searchpath := a.search
	if a.searchFunc != nil {
		searchpath, err = a.searchFunc(state)
		if err != nil {
			return middleware.NextOrFailure(a.Name(), a.Next, ctx, w, r)
		}
		fmt.Printf("search from chaos %v\n", searchpath)
	}

	do := a.FirstInSearchPath(state.Name())
	if !do { // Does not fall in the search path, normal middleware chaining
		// TOD(miek): should actually have a fallthrough thing for this; should be yes.
		return middleware.NextOrFailure(a.Name(), a.Next, ctx, w, r)
	}

	origQName := state.QName()

	// Establish base name of the query. I.e what was originally asked.
	base, err := dnsutil.TrimZone(state.QName(), a.search[0]) // TOD(miek): we loose the original case of the query here.
	if err != nil {
		return dns.RcodeServerFailure, err
	}
	println("DOING AUTOPATH")

	firstReply := new(dns.Msg)
	firstRcode := 0
	var firstErr error
	// Walk the search path and see if we can get a non-domain - if they all fail we return the first
	// query we've done and return that as-is.
	for i, s := range a.search {

		newQName := base + "." + s
		r.Question[0].Name = newQName
		nw := NewNonWriter(w)

		rcode, err := middleware.NextOrFailure(a.Name(), a.Next, ctx, nw, r)
		if i == 0 {
			firstReply = nw.Msg
			firstRcode = rcode
			firstErr = err
		}

		if nw.Msg.Rcode == dns.RcodeNameError {
			println("continuing")
			continue
		}

		msg := nw.Msg
		cnamer(msg, origQName)

		// Other failure or successful answer - should check what we need to here
		w.WriteMsg(msg)
		return rcode, err

	}
	println("ALL NXDOMAIN, returning first")
	// Still here, return first reply
	w.WriteMsg(firstReply)
	return firstRcode, firstErr
}

// FirstInSearchPath checks if name is equal to are a sibling of the first element
// in the search path.
func (a *AutoPath) FirstInSearchPath(name string) bool {
	if name == a.search[0] {
		return true
	}
	if dns.IsSubDomain(a.search[0], name) {
		return true
	}
	return false
}

func (a AutoPath) Name() string { return "autopath" }
