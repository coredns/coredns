//go:build coredns_all || coredns_cache

package cache

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
