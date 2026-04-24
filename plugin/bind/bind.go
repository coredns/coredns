//go:build coredns_all || coredns_bind

// Package bind allows binding to a specific interface instead of bind to all of them.
package bind

import (
	"github.com/coredns/coredns/plugin"
)

type bind struct {
	Next   plugin.Handler
	addrs  []string
	except []string
}

// Name implements plugin.Handler.
func (b *bind) Name() string { return "bind" }
