package dnsserver

import (
	"context"

	"github.com/coredns/coredns/request"
)

// Viewer - If Viewer is implemented by a plugin in a server block, its Filter()
// is added to the server block's filter functions when starting the server. When a running server
// serves a DNS request, it will route the request to the first Config (server block) that passes
// all its filter functions.
type Viewer interface {
	Filter(ctx context.Context, req *request.Request) bool
}
