//go:build coredns_all || coredns_nsid

package nsid

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
