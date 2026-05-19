//go:build coredns_all || coredns_reload

package reload

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
