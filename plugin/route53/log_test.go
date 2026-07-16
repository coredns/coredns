//go:build coredns_all || coredns_route53

package route53

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
