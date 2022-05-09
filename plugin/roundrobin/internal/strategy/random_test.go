package strategy

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestRoundRobinIsSwitchingCorrectly(t *testing.T) {
	const maxAttemptsToReachVariation = 100
	var avariations = []string{"[10.240.0.1 10.240.0.2 10.240.0.3]", "[10.240.0.1 10.240.0.3 10.240.0.2]",
		"[10.240.0.3 10.240.0.2 10.240.0.1]", "[10.240.0.3 10.240.0.1 10.240.0.2]",
		"[10.240.0.2 10.240.0.1 10.240.0.3]", "[10.240.0.2 10.240.0.3 10.240.0.1]"}

	var aaaavariations = []string{"[4001:a1:1014::89 4001:a1:1014::8a 4001:a1:1014::8b]", "[4001:a1:1014::89 4001:a1:1014::8b 4001:a1:1014::8a]",
		"[4001:a1:1014::8b 4001:a1:1014::8a 4001:a1:1014::89]", "[4001:a1:1014::8b 4001:a1:1014::89 4001:a1:1014::8a]",
		"[4001:a1:1014::8a 4001:a1:1014::89 4001:a1:1014::8b]", "[4001:a1:1014::8a 4001:a1:1014::8b 4001:a1:1014::89]"}

	tests := []struct {
		name             string
		rr               []dns.RR
		expectedResponse []string
	}{
		{"A and non A records",
			[]dns.RR{
				test.CNAME("alpha.cloud.example.com.	300	IN	CNAME		beta.cloud.example.com."),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.3"),
				test.MX("alpha.cloud.example.com.			300	IN	MX		1	mxa-alpha.cloud.example.com.")},
			avariations,
		},
		{
			"A records only",
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.3")},
			avariations,
		},
		{
			"AAAA records only",
			[]dns.RR{
				test.AAAA("ipv6.cloud.example.com.		300	IN	AAAA			4001:a1:1014::89"),
				test.AAAA("ipv6.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8a"),
				test.AAAA("ipv6.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8b"),
			},
			aaaavariations,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newMid()
			m.res.Answer = test.rr
			for _, v := range test.expectedResponse {
				var x int
				for x = 0; x < maxAttemptsToReachVariation; x++ {
					result, _ := NewRandom().Shuffle(m.req, m.res)
					if fmt.Sprintf("%v", getIPs(result)) == v {
						break
					}
				}
				if x == maxAttemptsToReachVariation {
					t.Errorf("The Random shuffle is not working as expected. %v didn't occure during %v attempts", v, x)
				}
			}
		})
	}
}

func TestRoundRobinRandomEmptyAnswer(t *testing.T) {
	m := newMid()
	result, _ := NewRandom().Shuffle(m.req, m.res)
	if len(result) != 0 {
		t.Errorf("Expecting empty result but got %v", result)
	}
}

func TestRoundRobinRandomOrderForAandNonA(t *testing.T) {
	m := newMid()
	m.AddResponseAnswer(test.CNAME("alpha.cloud.example.com.	300	IN	CNAME		beta.cloud.example.com."))
	m.AddResponseAnswer(test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"))
	result, _ := NewRandom().Shuffle(m.req, m.res)
	if result[0].String() != m.res.Answer[1].String() {
		t.Errorf("Expecting %s result but got %s", result[0].String(), m.res.Answer[1].String())
	}
	if result[1].String() != m.res.Answer[0].String() {
		t.Errorf("Expecting %s result but got %s", result[1].String(), m.res.Answer[0].String())
	}
}

func TestRoundRobinRandomStableOrderForNonA(t *testing.T) {
	m := newMid()
	m.AddResponseAnswer(test.CNAME("alpha.cloud.example.com.	300	IN	CNAME		beta.cloud.example.com."))
	m.AddResponseAnswer(test.MX("alpha.cloud.example.com.			300	IN	MX		1	mxa-alpha.cloud.example.com."))
	m.AddResponseAnswer(test.MX("alpha.cloud.example.com.			300	IN	MX		1	mx-beta.cloud.example.com."))
	result, _ := NewRandom().Shuffle(m.req, m.res)
	if len(result) != 3 {
		t.Errorf("Expecting %v result but got %v", len(m.res.Answer), len(result))
	}
	for i, v := range result {
		if v.String() != m.res.Answer[i].String() {
			t.Errorf("Expecting %s result but got %s", v.String(), m.res.Answer[i].String())
		}
	}
}
