//go:build coredns_all || coredns_whoami

package whoami

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
