package strategy

import (
	"fmt"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestStatefulGCCleaning(t *testing.T) {
	flattenTests := []stateFlatten{
		{"10.20.30.40", "test.example.com.", dnsTypes.A, time.Now().Add(time.Hour * -5), []string{"10.10.10.10"}},
		{"10.20.30.40", "alpha.example.com.", dnsTypes.A, time.Now().Add(time.Minute * -5), []string{"10.10.10.10", "20.20.20.20"}},
	}
	tests := []struct {
		name       string
		ttlSeconds int
		state      mstate
	}{
		{"clean on empty", 5, mstate{}},
		{"clean all records", 5, buildState(flattenTests)},
		{"nil state", 5, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			newGarbageCollector(test.state, test.ttlSeconds).collect()
			if len(test.state) != 0 {
				t.Fatalf("Expected empty state but have %v records", len(test.state))
			}
		})
	}
}

func TestStatefulGCCleaningLive(t *testing.T) {
	flattenTests := []stateFlatten{
		{"10.20.30.40", "alpha.example.com.", dnsTypes.A, time.Now().Add(time.Hour * -5), []string{"10.10.10.10", "20.20.20.20"}},
	}
	tests := []struct {
		name     string
		question string
		dnstype  dnsType
		from     string
		answer   []dns.RR
	}{
		{"Retrieving records with old timestamp", "alpha.example.com.", dnsTypes.A, "10.20.30.40",
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.10.10.10"),
				test.A("alpha.cloud.example.com.		300	IN	A			20.20.20.20")}},
		{"Call once again", "alpha.example.com.", dnsTypes.A, "10.20.30.40",
			[]dns.RR{
				test.A("alpha.cloud.example.com.		300	IN	A			10.10.10.10"),
				test.A("alpha.cloud.example.com.		300	IN	A			20.20.20.20")}},
	}
	s := NewStateful()
	s.state.state = buildState(flattenTests)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newMid()
			m.SetQuestion(test.question, dns.TypeA)
			m.SetSubnet(test.from)
			m.res.Answer = test.answer
			ts := s.state.state[key(test.from)][question(test.question)][test.dnstype].timestamp
			_, _ = s.Shuffle(m.req, m.res)

			if !s.state.state[key(test.from)][question(test.question)][test.dnstype].timestamp.After(ts) {
				t.Fatalf("timestamp has not been properly set")
			}
		})
	}
}

func TestStatefulGCRemoveItem(t *testing.T) {
	flattenTests := []stateFlatten{
		{"10.20.30.40", "test.example.com.", dnsTypes.A, time.Now().Add(time.Hour * -5), []string{"10.10.10.10"}},
		{"10.20.30.40", "alpha.example.com.", dnsTypes.A, time.Now().Add(time.Minute * -5), []string{"10.10.10.10", "20.20.20.20"}},
		{"10.20.30.40", "beta.example.com.", dnsTypes.A, time.Now().Add(time.Second * -5), []string{}},
		{"10.20.30.40", "beta.example.com.", dnsTypes.A, time.Now().Add(time.Second * -1), []string{"11.111.111.111", "222.222.222.333"}},
		{"11.11.11.11", "gc.test.com.", dnsTypes.A, time.Now(), []string{"10.10.10.10"}},
	}

	tests := []struct {
		state              []stateFlatten
		ttlSeconds         int
		survivalRowIndexes map[int]bool
	}{
		{flattenTests, 3600*5 + 1, map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true}},
		{flattenTests, 60 * 5, map[int]bool{0: false, 1: false, 2: true, 3: true, 4: true}},
		{flattenTests, 60*5 + 1, map[int]bool{0: false, 1: true, 2: true, 3: true, 4: true}},
		{flattenTests, 2, map[int]bool{0: false, 1: false, 2: true, 3: true, 4: true}},
		{flattenTests, 1, map[int]bool{0: false, 1: false, 2: false, 3: false, 4: true}},
		{flattenTests, 0, map[int]bool{0: false, 1: false, 2: false, 3: false, 4: false}},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("Delete records older than %v seconds", test.ttlSeconds), func(t *testing.T) {
			s := buildState(test.state)
			newGarbageCollector(s, test.ttlSeconds).collect()

			for i, v := range flattenTests {
				// check if state for key x question exists
				exists := s.exists(key(v.key), question(v.question), v.t)

				if test.survivalRowIndexes[i] != exists {
					t.Fatalf("Inconsistent state. Check if %v should be there or not ", v)
				}
			}
		})
	}
}
