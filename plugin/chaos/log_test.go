//go:build coredns_all || coredns_chaos

package chaos

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
