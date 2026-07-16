//go:build coredns_all || coredns_health

package health

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
