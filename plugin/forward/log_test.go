//go:build coredns_all || coredns_forward

package forward

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
