package cache

import (
	"context"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/request"
)

// TODO: REMOVE. This is just for interactive testing.
func (c *Cache) Metadata(ctx context.Context, state request.Request) context.Context {
	metadata.SetValueFunc(ctx, "cache/test1", func() string {
		return "foo"
	})
	metadata.SetValueFunc(ctx, "cache/test2", func() string {
		return "bar"
	})

	return ctx
}
