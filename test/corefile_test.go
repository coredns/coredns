package test

import (
	"testing"

	"github.com/miekg/dns"
)

// TestCorefileParsing tests the Corefile parsing functionality.
// Expected to not panic or timeout.
func TestCorefileParsing(t *testing.T) {
	cases := []struct {
		name     string
		corefile string
	}{
		{
			// See: https://github.com/coredns/coredns/pull/4637
			name: "PR4637_" + "NoPanicOnEscapedBackslashesAndUnicode",
			corefile: `\\\\ȶ.
acl
`,
		},
		{
			// See: https://github.com/coredns/coredns/pull/7571
			name: "PR7571_" + "InvalidBlockFailsToStart",
			corefile: "\xD9//\n" +
				"hosts#\x90\xD0{lc\x0C{\n" +
				"'{mport\xEF1\x0C}\x0B''",
		},
		{
			// A kubernetes endpoint URL with invalid UTF-8 caused a
			// panic in Prometheus WithLabelValues.
			// See OSS-Fuzz issue: https://issues.oss-fuzz.com/issues/498472468
			name: "FuzzCore_InvalidUTF8InKubernetesEndpoint",
			corefile: "\xf6\xe6*S65558::65535\n" +
				"kubernetes idd\x0cd\xc8:0\x00,\x13" +
				"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xfd" +
				"\x00\x00\x00\x00\x00\x00\x00-\x00\x00\x00\x00" +
				"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" +
				"\t{\tendpoint m\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
				"\xff\xff\xff\xffFFFFFF%FFFFFFFF\xff\xff\xff\xff\xff" +
				"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
				"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
				"\xff\xff\tuil{ticll{ticl\x00,1:*\x0cd}\x0c}",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Expected no panic, but got %v", r)
				}
			}()

			i, _, _, _ := CoreDNSServerAndPorts(tc.corefile)

			defer func() {
				if i != nil {
					i.Stop()
				}
			}()
		})
	}
}

func TestUppercaseServerBlockZone(t *testing.T) {
	instance, udp, _, err := CoreDNSServerAndPorts(`EXAMPLE.ORG.:0 {
	whoami
}`)
	if err != nil {
		t.Fatalf("failed to start CoreDNS: %v", err)
	}
	defer CoreDNSServerStop(instance)

	query := new(dns.Msg)
	query.SetQuestion("www.example.org.", dns.TypeA)
	response, _, err := new(dns.Client).Exchange(query, udp)
	if err != nil {
		t.Fatalf("DNS exchange failed: %v", err)
	}
	if response.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected response code %s, got %s", dns.RcodeToString[dns.RcodeSuccess], dns.RcodeToString[response.Rcode])
	}
}
