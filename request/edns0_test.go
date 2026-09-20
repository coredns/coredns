package request

import (
	"testing"

	"github.com/coredns/coredns/plugin/pkg/edns"

	"github.com/miekg/dns"
)

func TestSupportedOptions(t *testing.T) {
	const supportedCode = 65001
	edns.SetSupportedOption(supportedCode)

	want := &dns.EDNS0_LOCAL{Code: supportedCode}
	options := []dns.EDNS0{
		&dns.EDNS0_NSID{},
		&dns.EDNS0_EXPIRE{},
		&dns.EDNS0_COOKIE{},
		&dns.EDNS0_TCP_KEEPALIVE{},
		&dns.EDNS0_PADDING{},
		&dns.EDNS0_LOCAL{Code: supportedCode + 1},
		want,
	}

	got := supportedOptions(options)
	if len(got) != 1 {
		t.Fatalf("Expected one explicitly supported option, got %d: %v", len(got), got)
	}
	if got[0] != want {
		t.Errorf("Expected explicitly supported option %v, got %v", want, got[0])
	}
}
