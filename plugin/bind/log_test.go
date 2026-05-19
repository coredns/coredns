//go:build coredns_all || coredns_bind

package bind

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
