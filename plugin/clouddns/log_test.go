//go:build coredns_all || coredns_clouddns

package clouddns

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
