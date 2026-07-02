//go:build coredns_all || coredns_metadata || coredns_cache

package metadata

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
