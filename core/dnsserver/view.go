package dnsserver

import "github.com/coredns/coredns/request"

// Viewer - If Viewer is implemented by a registered plugin in a server, its Filter()
// is added to the server's filterFuncs.
type Viewer interface {
	Filter(request.Request) bool
}
