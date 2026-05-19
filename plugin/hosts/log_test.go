//go:build coredns_all || coredns_hosts

package hosts

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
