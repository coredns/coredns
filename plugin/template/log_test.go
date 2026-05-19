//go:build coredns_all || coredns_template

package template

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
