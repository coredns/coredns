package request

import (
	"github.com/coredns/coredns/plugin/pkg/edns"

	"github.com/miekg/dns"
)

func supportedOptions(o []dns.EDNS0) []dns.EDNS0 {
	supported := make([]dns.EDNS0, 0, 3)
	for _, opt := range o {
		if edns.SupportedOption(opt.Option()) {
			supported = append(supported, opt)
		}
	}
	return supported
}
