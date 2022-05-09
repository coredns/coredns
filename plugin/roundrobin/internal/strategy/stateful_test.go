package strategy

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestRoundRobinStatefulShuffle(t *testing.T) {
	tests := []struct {
		name                         string
		question                     string
		dnsType                      uint16
		expectedResults              []string
		answer                       []dns.RR
		expectedNonAPositionsMapping map[int]int
	}{
		{
			name:     "A and non A records",
			question: "alpha.cloud.example.com.", dnsType: dns.TypeA,
			expectedResults: []string{"[10.240.0.2 10.240.0.3 10.240.0.4 10.240.0.1]", "[10.240.0.3 10.240.0.4 10.240.0.1 10.240.0.2]",
				"[10.240.0.4 10.240.0.1 10.240.0.2 10.240.0.3]", "[10.240.0.1 10.240.0.2 10.240.0.3 10.240.0.4]"},
			answer: []dns.RR{
				test.CNAME("alpha.cloud.example.com.	300	IN	CNAME		beta.cloud.example.com."),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.3"),
				test.MX("alpha.cloud.example.com.			300	IN	MX		1	mxa-alpha.cloud.example.com."),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.4"),
			},
			expectedNonAPositionsMapping: map[int]int{0: 4, 4: 5},
		},
		{
			name:     "A records only",
			question: "alpha.cloud.example.com.", dnsType: dns.TypeA,
			expectedResults: []string{"[10.240.0.2 10.240.0.3 10.240.0.4 10.240.0.1]", "[10.240.0.3 10.240.0.4 10.240.0.1 10.240.0.2]",
				"[10.240.0.4 10.240.0.1 10.240.0.2 10.240.0.3]", "[10.240.0.1 10.240.0.2 10.240.0.3 10.240.0.4]"},
			answer: []dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.3"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.4"),
			},
			expectedNonAPositionsMapping: map[int]int{},
		},
		{
			name:     "AAAA and non AAAA records",
			question: "alpha.cloud.example.com.", dnsType: dns.TypeA,
			expectedResults: []string{"[4001:a1:1014::8a 4001:a1:1014::8b 4001:a1:1014::89]", "[4001:a1:1014::8b 4001:a1:1014::89 4001:a1:1014::8a]",
				"[4001:a1:1014::89 4001:a1:1014::8a 4001:a1:1014::8b]"},
			answer: []dns.RR{
				test.AAAA("alpha.cloud.example.com.		300	IN	AAAA			4001:a1:1014::89"),
				test.CNAME("alpha.cloud.example.com.	300	IN	CNAME		beta.cloud.example.com."),
				test.MX("alpha.cloud.example.com.			300	IN	MX		1	mxa-alpha.cloud.example.com."),
				test.AAAA("alpha.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8a"),
				test.AAAA("alpha.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8b"),
			},
			expectedNonAPositionsMapping: map[int]int{1: 3, 2: 4},
		},
		{
			name:     "Empty answers",
			question: "alpha.cloud.example.com.", dnsType: dns.TypeA,
			expectedResults:              []string{},
			answer:                       []dns.RR{},
			expectedNonAPositionsMapping: map[int]int{},
		},
		{
			name:     "Nil answers",
			question: "alpha.cloud.example.com.", dnsType: dns.TypeA,
			expectedResults:              []string{},
			answer:                       []dns.RR{},
			expectedNonAPositionsMapping: map[int]int{},
		},
		{
			name:     "One A record",
			question: "alpha.cloud.example.com.", dnsType: dns.TypeA,
			expectedResults: []string{"[10.240.0.1]"},
			answer: []dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
			},
			expectedNonAPositionsMapping: map[int]int{},
		},
		{
			name:     "One A record and several non A records",
			question: "alpha.cloud.example.com.", dnsType: dns.TypeA,
			expectedResults: []string{"[10.240.0.1]"},
			answer: []dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.MX("alpha.cloud.example.com.			300	IN	MX		1	mxa-alpha.cloud.example.com."),
				test.CNAME("alpha.cloud.example.com.	300	IN	CNAME		beta.cloud.example.com."),
			},
			expectedNonAPositionsMapping: map[int]int{1: 1, 2: 2},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var s = NewStateful()
			// requesting several times and check if rotation works
			for i := 0; i < 10; i++ {
				m := newMid()
				m.SetQuestion(test.question, test.dnsType)
				m.res.Answer = test.answer
				clientState, e := s.Shuffle(m.req, m.res)

				if e != nil {
					t.Errorf("unexpepcted error %s", e)
				}

				if len(test.expectedResults) > 0 && fmt.Sprintf("%v", getIPs(clientState)) !=
					fmt.Sprintf("%v", test.expectedResults[i%len(test.expectedResults)]) {
					t.Errorf("%v: The stateful rotation is not working. Expecting %v but got %v.",
						i, test.expectedResults[i%len(test.expectedResults)], getIPs(clientState))
				}

				if len(clientState) != len(m.res.Answer) {
					t.Errorf("The stateful retrieved different number of records. Expected %v got %v",
						len(m.res.Answer), len(clientState))
				}

				// If clientState contains non A, we check it exists on specified positions
				for originalPosition, newPosition := range test.expectedNonAPositionsMapping {
					if clientState[newPosition].String() != test.answer[originalPosition].String() {
						t.Errorf("Expecting %s result but got %s", test.answer[originalPosition], clientState[newPosition])
					}
				}
			}
		})
	}
}

func TestRoundRobinStatefulNoQuestion(t *testing.T) {
	m := newMid()
	clientState, e := NewStateful().Shuffle(m.req, m.res)
	if e == nil {
		t.Errorf("expecting error")
	}
	if len(clientState) != 0 {
		t.Errorf("The stateful retrieved different number of records. Expected %v got %v", len(m.res.Answer), 0)
	}
}

func TestRoundRobinStatefulState(t *testing.T) {
	s := NewStateful()
	tests := []struct {
		question    string
		from        string
		expectedKey string
		setSubnet   bool
		rr          []dns.RR
	}{
		{"alpha.cloud.example.com.", "200.10.0.0", "200.10.0.0", true,
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2")}},
		{"alpha.cloud.example.com.", "101.203.0.0", "101.203.0.0", true,
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2")}},
		{"beta.cloud.example.com.", "101.203.0.0", "101.203.0.0", true,
			[]dns.RR{
				test.A("beta.cloud.example.com.		300	IN	A			20.100.0.1")}},
		{"beta.cloud.example.com.", "102.203.0.0", "102.203.0.0", true,
			[]dns.RR{
				test.A("beta.cloud.example.com.		300	IN	A			20.100.0.1")}},
		{"ipv6-subnet.cloud.example.com.", "4001:a1:1014::8a", "4001:a1:1014::8a", true,
			[]dns.RR{
				test.A("ipv6.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("ipv6.cloud.example.com.		300	IN	A			10.240.0.2"),
				test.A("ipv6.cloud.example.com.		300	IN	A			10.240.0.3")}},
		{"no-records.cloud.example.com.", "200.10.0.0", "200.10.0.0", true, []dns.RR{}},
		{"empty-subnet-and-empty-response.cloud.com.", "", emptySubnet, true, []dns.RR{}},
		{"empty-subnet-with-a-records.cloud.com.", "", emptySubnet, true, []dns.RR{
			test.A("empty-subnet-with-a-records.cloud.com.		300	IN	A			10.240.0.1"),
			test.A("empty-subnet-with-a-records.cloud.com.		300	IN	A			10.240.0.2"),
		}},
		{"missing-subnet-and-empty-response.cloud.com.", "", missingSubnet, false, []dns.RR{}},
		{"missing-subnet-with-a-records.cloud.com.", "", missingSubnet, false, []dns.RR{
			test.A("missing-subnet-with-a-records.cloud.com.		300	IN	A			10.240.0.1"),
			test.A("missing-subnet-with-a-records.cloud.com.		300	IN	A			10.240.0.2"),
		}},
		{"only-cname.cloud.example.com.", "200.10.0.0", "200.10.0.0", true, []dns.RR{
			test.CNAME("alpha.cloud.example.com.	300	IN	CNAME		beta.cloud.example.com."),
		}},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Request %s from %s", test.question, test.from), func(t *testing.T) {
			// arrange
			m := newMid()
			m.SetQuestion(test.question, dns.TypeA)
			if test.setSubnet {
				m.SetSubnet(test.from)
			}
			for _, a := range test.rr {
				m.AddResponseAnswer(a)
			}

			// act
			_, _ = s.Shuffle(m.req, m.res)

			// assert
			ipMap := ipsToSet(getIPs(test.rr))
			if len(s.state.state[key(test.expectedKey)][question(test.question)][dnsTypes.A].ip) != len(getIPs(test.rr)) {
				t.Errorf("the number of records in the test (%v) and the state (%v) do not match.",
					len(test.rr), len(s.state.state[key(test.from)][question(test.question)][dnsTypes.A].ip))
			}
			for _, ip := range s.state.state[key(test.expectedKey)][question(test.question)][dnsTypes.A].ip {
				if !ipMap[ip] {
					t.Errorf("Can't find %s for state[%s][%s] ", ip, test.from, test.question)
				}
			}
		})
	}
}

func TestRoundRobinStatefulDNSRecordsChange(t *testing.T) {
	tests := []struct {
		name           string
		question       string
		from           string
		rr             []dns.RR
		expectedResult []string
	}{
		{"Create records for alpha.cloud.example.com.",
			"alpha.cloud.example.com.", "200.10.0.0",
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.3"),
			},
			[]string{"10.240.0.2", "10.240.0.3", "10.240.0.1"},
		},
		{"Alter record for alpha.cloud.example.com.",
			"alpha.cloud.example.com.", "200.10.0.0",
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.3"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.4"),
			},
			[]string{"10.240.0.3", "10.240.0.1", "10.240.0.4", "10.240.0.2"},
		},
		{"Remove records for alpha.cloud.example.com.",
			"alpha.cloud.example.com.", "200.10.0.0",
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.4"),
			},
			[]string{"10.240.0.4", "10.240.0.1"},
		},
		{"Add non A records for alpha.cloud.example.com.",
			"alpha.cloud.example.com.", "200.10.0.0",
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.4"),
				test.CNAME("alpha.cloud.example.com.	300	IN	CNAME		beta.cloud.example.com."),
				test.MX("alpha.cloud.example.com.			300	IN	MX		1	mxa-alpha.cloud.example.com."),
			},
			[]string{"10.240.0.1", "10.240.0.4"},
		},
		{"Remove non A records for alpha.cloud.example.com.",
			"alpha.cloud.example.com.", "200.10.0.0",
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.4"),
			},
			[]string{"10.240.0.4", "10.240.0.1"},
		},
		{"Exchange records for alpha.cloud.example.com.",
			"alpha.cloud.example.com.", "200.10.0.0",
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			100.0.0.100"),
				test.A("alpha.cloud.example.com.		300	IN	A			100.0.0.200"),
			},
			[]string{"100.0.0.200", "100.0.0.100"},
		},
		{"No change.",
			"alpha.cloud.example.com.", "200.10.0.0",
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			100.0.0.100"),
				test.A("alpha.cloud.example.com.		300	IN	A			100.0.0.200"),
			},
			[]string{"100.0.0.100", "100.0.0.200"},
		},
		{"Remove records",
			"alpha.cloud.example.com.", "200.10.0.0",
			[]dns.RR{},
			[]string{},
		},
	}
	s := NewStateful()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// arrange
			m := newMid()
			m.SetQuestion(test.question, dns.TypeA)
			m.SetSubnet(test.from)
			for _, a := range test.rr {
				m.AddResponseAnswer(a)
			}

			// act
			clientState, e := s.Shuffle(m.req, m.res)

			// assert
			if e != nil {
				t.Errorf("unexpepcted error %s", e)
			}

			if len(test.rr) != len(clientState) {
				t.Errorf("The stateful retrieved different number of records. Expected %v got %v", len(test.rr), len(clientState))
			}

			if fmt.Sprintf("%v", getIPs(clientState)) != fmt.Sprintf("%v", test.expectedResult) {
				t.Errorf("The stateful rotation is not working. Expecting %v but got %v.", test.expectedResult, getIPs(clientState))
			}
		})
	}
}

func TestResolverBehaviorIPv4asFallbackOfIPv6(t *testing.T) {
	tests := []struct {
		name           string
		question       string
		from           string
		dnsType        uint16
		rr             []dns.RR
		expectedResult []string
	}{
		{"Resolver makes automatically AAAA request first but has no records",
			"alpha.cloud.example.com.", "200.10.0.0", dns.TypeAAAA,
			[]dns.RR{},
			[]string{},
		},
		{"Empty result fallbacks to A request and retrieves records in right order",
			"alpha.cloud.example.com.", "200.10.0.0", dns.TypeA,
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.3"),
			},
			[]string{"10.240.0.2", "10.240.0.3", "10.240.0.1"},
		},
		{"Resolver makes again AAAA request again and has no records",
			"alpha.cloud.example.com.", "200.10.0.0", dns.TypeAAAA,
			[]dns.RR{},
			[]string{},
		},
		{"Empty result fallbacks to A request and retrieves records in right order",
			"alpha.cloud.example.com.", "200.10.0.0", dns.TypeA,
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.3"),
			},
			[]string{"10.240.0.3", "10.240.0.1", "10.240.0.2"},
		},
		{"Resolver makes again AAAA request again and has no records",
			"alpha.cloud.example.com.", "200.10.0.0", dns.TypeAAAA,
			[]dns.RR{},
			[]string{},
		},
		{"Someone makes  AAAA request and has no records",
			"alpha.cloud.example.com.", "200.10.0.0", dns.TypeAAAA,
			[]dns.RR{},
			[]string{},
		},
		{"Empty result fallbacks to A request and retrieves records in right order",
			"alpha.cloud.example.com.", "200.10.0.0", dns.TypeA,
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.3"),
			},
			[]string{"10.240.0.1", "10.240.0.2", "10.240.0.3"},
		},
		{"Doing A request only and retrieves records in right order",
			"alpha.cloud.example.com.", "200.10.0.0", dns.TypeA,
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.3"),
			},
			[]string{"10.240.0.2", "10.240.0.3", "10.240.0.1"},
		},
		{"Resolver makes AAAA request and records successfully retrieved",
			"beta.cloud.example.com.", "200.11.0.0", dns.TypeAAAA,
			[]dns.RR{
				test.AAAA("beta.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8a"),
				test.AAAA("beta.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8b"),
				test.AAAA("beta.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8c"),
			},
			[]string{"4001:a1:1014::8b", "4001:a1:1014::8c", "4001:a1:1014::8a"},
		},
		{"Resolver makes AAAA request and records successfully retrieved",
			"beta.cloud.example.com.", "200.11.0.0", dns.TypeAAAA,
			[]dns.RR{
				test.AAAA("beta.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8a"),
				test.AAAA("beta.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8b"),
				test.AAAA("beta.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8c"),
			},
			[]string{"4001:a1:1014::8c", "4001:a1:1014::8a", "4001:a1:1014::8b"},
		},
		{"Someone makes A request and records successfully retrieved",
			"beta.cloud.example.com.", "200.11.0.0", dns.TypeA,
			[]dns.RR{
				test.A("beta.cloud.example.com.		300	IN	A			1.1.1.1"),
				test.A("beta.cloud.example.com.		300	IN	A			1.1.1.2"),
			},
			[]string{"1.1.1.2", "1.1.1.1"},
		},
		{"Resolver makes AAAA request and records successfully retrieved",
			"beta.cloud.example.com.", "200.11.0.0", dns.TypeAAAA,
			[]dns.RR{
				test.AAAA("beta.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8a"),
				test.AAAA("beta.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8b"),
				test.AAAA("beta.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8c"),
			},
			[]string{"4001:a1:1014::8a", "4001:a1:1014::8b", "4001:a1:1014::8c"},
		},
		{"Someone makes A request and records successfully retrieved",
			"beta.cloud.example.com.", "200.11.0.0", dns.TypeA,
			[]dns.RR{
				test.A("beta.cloud.example.com.		300	IN	A			1.1.1.1"),
				test.A("beta.cloud.example.com.		300	IN	A			1.1.1.2"),
			},
			[]string{"1.1.1.1", "1.1.1.2"},
		},
	}
	s := NewStateful()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// arrange
			m := newMid()
			m.SetQuestion(test.question, test.dnsType)
			m.SetSubnet(test.from)
			for _, a := range test.rr {
				m.AddResponseAnswer(a)
			}

			// act
			clientState, e := s.Shuffle(m.req, m.res)

			// assert
			if e != nil {
				t.Errorf("unexpepcted error %s", e)
			}

			if len(test.rr) != len(clientState) {
				t.Errorf("The stateful retrieved different number of records. Expected %v got %v", len(test.rr), len(clientState))
			}

			if fmt.Sprintf("%v", getIPs(clientState)) != fmt.Sprintf("%v", test.expectedResult) {
				t.Errorf("The stateful rotation is not working. Expecting %v but got %v.", test.expectedResult, getIPs(clientState))
			}
		})
	}
}
