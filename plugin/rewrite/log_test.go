//go:build coredns_all || coredns_rewrite

package rewrite

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
