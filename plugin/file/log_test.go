//go:build coredns_all || coredns_file || coredns_auto || coredns_secondary || coredns_route53 || coredns_azure || coredns_clouddns || coredns_sign

package file

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
