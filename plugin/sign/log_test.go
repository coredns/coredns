//go:build coredns_all || coredns_sign

package sign

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
