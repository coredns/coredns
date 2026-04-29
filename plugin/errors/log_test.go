//go:build coredns_all || coredns_errors

package errors

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
