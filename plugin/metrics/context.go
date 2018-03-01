package metrics

import (
	"github.com/coredns/coredns/plugin"

	"golang.org/x/net/context"
)

// WithServer returns the current server handling the request.
// It returns the server listening address which is something like "dns://:53".
// TODO(miek): take bind into account.
func WithServer(ctx context.Context) string {
	srv := ctx.Value(plugin.ServerCtx{})
	if srv == nil {
		return ""
	}
	return srv.(string)
}
