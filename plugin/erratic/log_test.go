//go:build coredns_all || coredns_erratic

package erratic

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
