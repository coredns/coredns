//go:build coredns_all || coredns_tls

package tls

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
