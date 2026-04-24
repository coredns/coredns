//go:build coredns_all || coredns_prometheus || coredns_auto

package metrics

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
