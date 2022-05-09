package strategy

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// In stateless, the client sends back previous state to the CoreDNS per DNS query,
// the server doesn't know anything about client's data.
// This test examines the response, different situations when we send different
// types of data to the server (uninitialized data or valid or invalid) and tests that we get the expected response.
func TestRoundRobinStatelessInitialize(t *testing.T) {
	expected := "[10.240.0.2 10.240.0.3 10.240.0.4 10.240.0.1]"
	cname := test.CNAME("alpha.cloud.example.com.	300	IN	CNAME		beta.cloud.example.com.")
	mx := test.MX("alpha.cloud.example.com.			300	IN	MX		1	mxa-alpha.cloud.example.com.")
	testValues := []struct {
		value    string
		expected string
	}{
		{"_rr_state=invalid", expected},
		{`_rr_state=`, expected},
		{``, expected},
		{`_rr_state=nil`, expected},
		{`_rr_state={}`, expected},
		{`_rr_state={"ip":[]}`, expected},
		{`_rr_state={"ip":["10.240.0.1"]}`, expected},
		{`_rr_state={"ip":[10.240.0.2 10.240.0.3 10.240.0.4 10.240.0.1]}`, expected},
		{`_rr_state={"ip":[10.240.0.10 10.240.0.20 10.240.0.40 10.240.0.111]}`, expected},
		{`_rr_state={"ip":["10.0.0.1","10.2.2.1","10.1.1.2","10.1.1.3","10.2.2.2","10.2.2.3","10.0.0.2","10.0.0.3","10.1.1.1"]}`, expected},
		{`blah=`, expected},
	}
	for _, raw := range testValues {
		t.Run(fmt.Sprintf("with%s", raw.value), func(t *testing.T) {
			m := newMid()

			m.AddResponseAnswer(cname)
			m.AddResponseAnswer(test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"))
			m.AddResponseAnswer(test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2"))
			m.AddResponseAnswer(test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.3"))
			m.AddResponseAnswer(test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.4"))
			m.AddResponseAnswer(mx)
			m.AddRequestOptRaw(raw.value)

			var clientState, err = NewStateless().Shuffle(m.req, m.res)

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(clientState) != len(m.res.Answer) {
				t.Errorf("The stateless retrieved different number of records. Expected %v got %v", len(m.res.Answer), len(clientState))
			}

			if fmt.Sprintf("%v", getIPs(clientState)) != raw.expected {
				t.Errorf("The stateless shuffle is not working as expected. For %s Expecting %v but got %v.", raw.value, expected, getIPs(clientState))
			}

			// end of the list contains additional records in the order of they are defined in the response
			if clientState[4].String() != cname.String() {
				t.Errorf("Expecting %s result but got %s", cname, clientState[4].String())
			}
			if clientState[5].String() != mx.String() {
				t.Errorf("Expecting %s result but got %s", mx, clientState[5].String())
			}
		})
	}
}

func TestRoundRobinStatelessShuffle(t *testing.T) {
	tests := []struct {
		name                         string
		answer                       []dns.RR
		expectedResponse             []string
		expectedNonAPositionsMapping map[int]int
		requestOpt                   []dns.RR
	}{
		{"A records",
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.2"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.3"),
				test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.4"),
			}, []string{"[10.240.0.2 10.240.0.3 10.240.0.4 10.240.0.1]", "[10.240.0.3 10.240.0.4 10.240.0.1 10.240.0.2]",
				"[10.240.0.4 10.240.0.1 10.240.0.2 10.240.0.3]", "[10.240.0.1 10.240.0.2 10.240.0.3 10.240.0.4]"},
			map[int]int{},
			[]dns.RR{},
		},
		{
			"AAAA and Non AAAA records",
			[]dns.RR{
				test.AAAA("ipv6.cloud.example.com.		300	IN	AAAA			4001:a1:1014::89"),
				test.CNAME("ipv6.cloud.example.com.	300	IN	CNAME		beta.cloud.example.com."),
				test.AAAA("ipv6.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8a"),
				test.AAAA("ipv6.cloud.example.com.		300	IN	AAAA			4001:a1:1014::8b"),
				test.MX("ipv6.cloud.example.com.			300	IN	MX		1	mxa-ipv6.cloud.example.com."),
			},
			[]string{"[4001:a1:1014::8a 4001:a1:1014::8b 4001:a1:1014::89]", "[4001:a1:1014::8b 4001:a1:1014::89 4001:a1:1014::8a]",
				"[4001:a1:1014::89 4001:a1:1014::8a 4001:a1:1014::8b]"},
			map[int]int{1: 3, 4: 4},
			[]dns.RR{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for i := 0; i < 10; i++ {
				m := newMid()
				m.res.Answer = test.answer
				// state, from the previous loop that arrived in the DNS query
				m.AddRequestOpt(test.requestOpt...)
				var clientState, err = NewStateless().Shuffle(m.req, m.res)

				// save the new state for the next query
				test.requestOpt = filterAandAAAA(clientState)

				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if fmt.Sprintf("%v", getIPs(clientState)) != fmt.Sprintf("%v", test.expectedResponse[i%len(test.expectedResponse)]) {
					t.Errorf("%v: The stateless rotation is not working. Expecting %v but got %v.",
						i, test.expectedResponse[i%len(test.expectedResponse)], getIPs(clientState))
				}

				if len(clientState) != len(m.res.Answer) {
					t.Errorf("The stateless retrieved different number of records. Expected %v got %v", len(m.res.Answer), len(clientState))
				}

				for originalPosition, newPosition := range test.expectedNonAPositionsMapping {
					if clientState[newPosition].String() != test.answer[originalPosition].String() {
						t.Errorf("Expecting %s result but got %s", test.answer[originalPosition], clientState[newPosition])
					}
				}
			}
		})
	}
}

func TestRoundRobinStatelessNoShuffle(t *testing.T) {
	tests := []struct {
		name             string
		request          string
		answer           []dns.RR
		expectedResponse []dns.RR
	}{
		{"answer is empty for any state",
			`_rr_state={"ip":[10.240.0.2 10.240.0.3 10.240.0.4 10.240.0.1]}`, []dns.RR{}, []dns.RR{}},
		{"answer is empty for empty state", ``, []dns.RR{}, []dns.RR{}},
		{"one record for any state", `_rr_state={"ip":[10.240.0.2 10.240.0.3 10.240.0.4 10.240.0.1]}`,
			[]dns.RR{test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1")},
			[]dns.RR{test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1")}},
		{"one record for empty state", "",
			[]dns.RR{test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1")},
			[]dns.RR{test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// arrange
			m := newMid()
			if len(test.request) != 0 {
				m.AddRequestOptRaw(test.request)
			}
			for _, a := range test.answer {
				m.AddResponseAnswer(a)
			}
			// act
			clientState, err := NewStateless().Shuffle(m.req, m.res)

			// assert
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if fmt.Sprintf("%v", getIPs(clientState)) != fmt.Sprintf("%v", getIPs(test.answer)) {
				t.Errorf("The stateless retrieved different number of records. Expected %v got %v", getIPs(test.answer), getIPs(clientState))
			}
		})
	}
}

func TestRoundRobinStatelessDNSRecordsChange(t *testing.T) {
	tests := []struct {
		name           string
		question       string
		request        string
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
	clientState := []dns.RR{}
	var err error
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// arrange
			m := newMid()
			m.SetQuestion(test.question, dns.TypeA)
			m.AddRequestOpt(clientState...)
			for _, a := range test.rr {
				m.AddResponseAnswer(a)
			}

			// act
			clientState, err = NewStateless().Shuffle(m.req, m.res)

			// assert
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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
