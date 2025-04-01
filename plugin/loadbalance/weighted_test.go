package loadbalance

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	testutil "github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

const oneDomainWRR = `
w1,example.org
192.168.1.15 10
192.168.1.14 20
`

var testOneDomainWRR = map[string]weights{
	"w1,example.org.": {
		&weightItem{net.ParseIP("192.168.1.15"), uint8(10)},
		&weightItem{net.ParseIP("192.168.1.14"), uint8(20)},
	},
}

const twoDomainsWRR = `
# domain 1
w1.example.org
192.168.1.15   10
192.168.1.14   20

# domain 2
w2.example.org
 # domain 3
 w3.example.org
 192.168.2.16 11
 192.168.2.15 12
 192.168.2.14 13
`

var testTwoDomainsWRR = map[string]weights{
	"w1.example.org.": {
		&weightItem{net.ParseIP("192.168.1.15"), uint8(10)},
		&weightItem{net.ParseIP("192.168.1.14"), uint8(20)},
	},
	"w2.example.org.": {},
	"w3.example.org.": {
		&weightItem{net.ParseIP("192.168.2.16"), uint8(11)},
		&weightItem{net.ParseIP("192.168.2.15"), uint8(12)},
		&weightItem{net.ParseIP("192.168.2.14"), uint8(13)},
	},
}

const missingWeightWRR = `
w1,example.org
192.168.1.14
192.168.1.15 20
`

const missingDomainWRR = `
# missing domain
192.168.1.14 10
w2,example.org
192.168.2.14 11
192.168.2.15 12
`

const wrongIpWRR = `
w1,example.org
192.168.1.300 10
`

const wrongWeightWRR = `
w1,example.org
192.168.1.14 300
`

const zeroWeightWRR = `
w1,example.org
192.168.1.14 0
`

func TestWeightFileUpdate(t *testing.T) {
	tests := []struct {
		weightFilContent   string
		shouldErr          bool
		expectedDomains    map[string]weights
		expectedErrContent string // substring from the expected error. Empty for positive cases.
	}{
		// positive
		{"", false, nil, ""},
		{oneDomainWRR, false, testOneDomainWRR, ""},
		{twoDomainsWRR, false, testTwoDomainsWRR, ""},
		// negative
		{missingWeightWRR, true, nil, "wrong domain name"},
		{missingDomainWRR, true, nil, "missing domain name"},
		{wrongIpWRR, true, nil, "wrong IP address"},
		{wrongWeightWRR, true, nil, "wrong weight value"},
		{zeroWeightWRR, true, nil, "wrong weight value"},
	}

	for i, test := range tests {
		testFile, rm, err := testutil.TempFile(".", test.weightFilContent)
		if err != nil {
			t.Fatal(err)
		}
		defer rm()
		weighted := &weightedRR{fileName: testFile}
		err = weighted.updateWeights()
		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found %s", i, err)
		}
		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found error: %v", i, err)
			}

			if !strings.Contains(err.Error(), test.expectedErrContent) {
				t.Errorf("Test %d: Expected error to contain: %v, found error: %v",
					i, test.expectedErrContent, err)
			}
		}
		if test.expectedDomains != nil {
			if len(test.expectedDomains) != len(weighted.domains) {
				t.Errorf("Test %d: Expected len(domains): %d but got %d",
					i, len(test.expectedDomains), len(weighted.domains))
			} else {
				_ = checkDomainsWRR(t, i, test.expectedDomains, weighted.domains)
			}
		}
	}
}

func checkDomainsWRR(t *testing.T, testIndex int, expectedDomains, domains map[string]weights) error {
	var ret error
	retError := errors.New("Check domains failed")
	for dname, expectedWeights := range expectedDomains {
		ws, ok := domains[dname]
		if !ok {
			t.Errorf("Test %d: Expected domain %s but not found it", testIndex, dname)
			ret = retError
		} else {
			if len(expectedWeights) != len(ws) {
				t.Errorf("Test %d: Expected len(weights): %d for domain %s but got %d",
					testIndex, len(expectedWeights), dname, len(ws))
				ret = retError
			} else {
				for i, w := range expectedWeights {
					if !w.address.Equal(ws[i].address) || w.value != ws[i].value {
						t.Errorf("Test %d: Weight list differs at index %d for domain %s. "+
							"Expected: %v got: %v", testIndex, i, dname, expectedWeights[i], ws[i])
						ret = retError
					}
				}
			}
		}
	}

	return ret
}

func TestPeriodicWeightUpdate(t *testing.T) {
	testFile1, rm, err := testutil.TempFile(".", oneDomainWRR)
	if err != nil {
		t.Fatal(err)
	}
	defer rm()
	testFile2, rm, err := testutil.TempFile(".", twoDomainsWRR)
	if err != nil {
		t.Fatal(err)
	}
	defer rm()

	// configure weightedRR with "oneDomainWRR" weight file content
	weighted := &weightedRR{fileName: testFile1}

	err = weighted.updateWeights()
	if err != nil {
		t.Fatal(err)
	} else {
		err = checkDomainsWRR(t, 0, testOneDomainWRR, weighted.domains)
		if err != nil {
			t.Fatalf("Initial check domains failed")
		}
	}

	// change weight file
	weighted.fileName = testFile2
	// start periodic update
	weighted.reload = 10 * time.Millisecond
	stopChan := make(chan bool)
	weighted.periodicWeightUpdate(stopChan)
	time.Sleep(20 * time.Millisecond)
	// stop periodic update
	close(stopChan)
	// check updated config
	weighted.mutex.Lock()
	err = checkDomainsWRR(t, 0, testTwoDomainsWRR, weighted.domains)
	weighted.mutex.Unlock()
	if err != nil {
		t.Fatalf("Final check domains failed")
	}
}

// Fake random number generator for testing
type fakeRandomGen struct {
	expectedLimit uint
	testIndex     int
	queryIndex    int
	randv         uint
	t             *testing.T
}

func (r *fakeRandomGen) randInit() {
}

func (r *fakeRandomGen) randUint(limit uint) uint {
	if limit != r.expectedLimit {
		r.t.Errorf("Test %d query %d: Expected weights sum %d but got %d",
			r.testIndex, r.queryIndex, r.expectedLimit, limit)
	}
	return r.randv
}

func TestLoadBalanceWRR(t *testing.T) {
	type testQuery struct {
		randv uint   // fake random value for selecting the top IP
		topIP string // top (first) address record in the answer
	}

	// domain maps to test
	oneDomain := map[string]weights{
		"endpoint.region2.skydns.test.": {
			&weightItem{net.ParseIP("10.240.0.2"), uint8(3)},
			&weightItem{net.ParseIP("10.240.0.1"), uint8(2)},
		},
	}
	twoDomains := map[string]weights{
		"endpoint.region2.skydns.test.": {
			&weightItem{net.ParseIP("10.240.0.2"), uint8(5)},
			&weightItem{net.ParseIP("10.240.0.1"), uint8(2)},
		},
		"endpoint.region1.skydns.test.": {
			&weightItem{net.ParseIP("::2"), uint8(4)},
			&weightItem{net.ParseIP("::1"), uint8(3)},
		},
	}

	// the first X records must be cnames after this test
	tests := []struct {
		answer        []dns.RR
		extra         []dns.RR
		cnameAnswer   int
		cnameExtra    int
		addressAnswer int
		addressExtra  int
		mxAnswer      int
		mxExtra       int
		domains       map[string]weights
		sumWeights    uint // sum of weights in the answer
		queries       []testQuery
	}{
		{
			answer: []dns.RR{
				testutil.CNAME("cname1.region2.skydns.test.	300	IN	CNAME		cname2.region2.skydns.test."),
				testutil.CNAME("cname2.region2.skydns.test.	300	IN	CNAME		cname3.region2.skydns.test."),
				testutil.CNAME("cname5.region2.skydns.test.	300	IN	CNAME		cname6.region2.skydns.test."),
				testutil.CNAME("cname6.region2.skydns.test.	300	IN	CNAME		endpoint.region2.skydns.test."),
				testutil.A("endpoint.region2.skydns.test.		300	IN	A			10.240.0.1"),
				testutil.A("endpoint.region2.skydns.test.	    300	IN	A			10.240.0.2"),
				testutil.A("endpoint.region2.skydns.test.	    300	IN	A			10.240.0.3"),
				testutil.AAAA("endpoint.region1.skydns.test.	300	IN	AAAA		::1"),
				testutil.AAAA("endpoint.region1.skydns.test.	300	IN	AAAA		::2"),
				testutil.MX("mx.region2.skydns.test.			300	IN	MX		1	mx1.region2.skydns.test."),
				testutil.MX("mx.region2.skydns.test.			300	IN	MX		2	mx2.region2.skydns.test."),
				testutil.MX("mx.region2.skydns.test.			300	IN	MX		3	mx3.region2.skydns.test."),
			},
			extra: []dns.RR{
				testutil.CNAME("cname6.region2.skydns.test.	300	IN	CNAME		endpoint.region2.skydns.test."),
				testutil.A("endpoint.region2.skydns.test.		300	IN	A			10.240.0.1"),
				testutil.A("endpoint.region2.skydns.test.	    300	IN	A			10.240.0.2"),
				testutil.A("endpoint.region2.skydns.test.	    300	IN	A			10.240.0.3"),
				testutil.AAAA("endpoint.region1.skydns.test.	300	IN	AAAA		::1"),
				testutil.AAAA("endpoint.region1.skydns.test.	300	IN	AAAA		::2"),
				testutil.MX("mx.region2.skydns.test.			300	IN	MX		1	mx1.region2.skydns.test."),
			},
			cnameAnswer:   4,
			cnameExtra:    1,
			addressAnswer: 5,
			addressExtra:  5,
			mxAnswer:      3,
			mxExtra:       1,
			domains:       twoDomains,
			sumWeights:    15,
			queries: []testQuery{
				{0, "10.240.0.2"},  // domain 1 weight 5
				{4, "10.240.0.2"},  // domain 1 weight 5
				{5, "::2"},         // domain 2 weight 4
				{8, "::2"},         // domain 2 weight 4
				{9, "::1"},         // domain 2 weight 3
				{11, "::1"},        // domain 2 weight 3
				{12, "10.240.0.1"}, // domain 1 weight 2
				{13, "10.240.0.1"}, // domain 1 weight 2
				{14, "10.240.0.3"}, // domain 1 no weight -> default weight
			},
		},
		{
			answer: []dns.RR{
				testutil.A("endpoint.region2.skydns.test.		300	IN	A			10.240.0.1"),
				testutil.MX("mx.region2.skydns.test.			300	IN	MX		1	mx1.region2.skydns.test."),
				testutil.CNAME("cname.region2.skydns.test.	300	IN	CNAME		endpoint.region2.skydns.test."),
				testutil.A("endpoint.region2.skydns.test.		300	IN	A			10.240.0.2"),
				testutil.A("endpoint.region1.skydns.test.		300	IN	A			10.240.0.3"),
			},
			cnameAnswer:   1,
			addressAnswer: 3,
			mxAnswer:      1,
			domains:       oneDomain,
			sumWeights:    6,
			queries: []testQuery{
				{0, "10.240.0.2"}, // weight 3
				{2, "10.240.0.2"}, // weight 3
				{3, "10.240.0.1"}, // weight 2
				{4, "10.240.0.1"}, // weight 2
				{5, "10.240.0.3"}, // no domain -> default weight
			},
		},
		{
			answer: []dns.RR{
				testutil.MX("mx.region2.skydns.test.			300	IN	MX		1	mx1.region2.skydns.test."),
				testutil.CNAME("cname.region2.skydns.test.	300	IN	CNAME		endpoint.region2.skydns.test."),
			},
			cnameAnswer: 1,
			mxAnswer:    1,
			domains:     oneDomain,
			queries: []testQuery{
				{0, ""}, // no address records -> answer unaltered
			},
		},
	}

	testRand := &fakeRandomGen{t: t}
	weighted := &weightedRR{randomGen: testRand}
	shuffle := func(res *dns.Msg) *dns.Msg {
		return weightedShuffle(res, weighted)
	}
	rm := LoadBalance{Next: handler(), shuffle: shuffle}

	rec := dnstest.NewRecorder(&testutil.ResponseWriter{})

	for i, test := range tests {
		// set domain map for weighted round robin
		weighted.domains = test.domains
		testRand.testIndex = i
		testRand.expectedLimit = test.sumWeights

		for j, query := range test.queries {
			req := new(dns.Msg)
			req.SetQuestion("endpoint.region2.skydns.test", dns.TypeSRV)
			req.Answer = test.answer
			req.Extra = test.extra

			// Set fake random number
			testRand.randv = query.randv
			testRand.queryIndex = j

			_, err := rm.ServeDNS(context.TODO(), rec, req)
			if err != nil {
				t.Errorf("Test %d: Expected no error, but got %s", i, err)
				continue
			}

			checkTopIP(t, i, j, rec.Msg.Answer, query.topIP)
			checkTopIP(t, i, j, rec.Msg.Extra, query.topIP)

			cname, address, mx, sorted := countRecords(rec.Msg.Answer)
			if query.topIP != "" && !sorted {
				t.Errorf("Test %d query %d: Expected CNAMEs, then AAAAs, then MX in Answer, but got mixed", i, j)
			}
			if cname != test.cnameAnswer {
				t.Errorf("Test %d query %d: Expected %d CNAMEs in Answer, but got %d", i, j, test.cnameAnswer, cname)
			}
			if address != test.addressAnswer {
				t.Errorf("Test %d query %d: Expected %d A/AAAAs in Answer, but got %d", i, j, test.addressAnswer, address)
			}
			if mx != test.mxAnswer {
				t.Errorf("Test %d query %d: Expected %d MXs in Answer, but got %d", i, j, test.mxAnswer, mx)
			}

			cname, address, mx, sorted = countRecords(rec.Msg.Extra)
			if query.topIP != "" && !sorted {
				t.Errorf("Test %d query %d: Expected CNAMEs, then AAAAs, then MX in Answer, but got mixed", i, j)
			}

			if cname != test.cnameExtra {
				t.Errorf("Test %d query %d: Expected %d CNAMEs in Extra, but got %d", i, j, test.cnameAnswer, cname)
			}
			if address != test.addressExtra {
				t.Errorf("Test %d query %d: Expected %d A/AAAAs in Extra, but got %d", i, j, test.addressAnswer, address)
			}
			if mx != test.mxExtra {
				t.Errorf("Test %d query %d: Expected %d MXs in Extra, but got %d", i, j, test.mxAnswer, mx)
			}
		}
	}
}

func checkTopIP(t *testing.T, i, j int, result []dns.RR, expectedTopIP string) {
	expected := net.ParseIP(expectedTopIP)
	for _, r := range result {
		switch r.Header().Rrtype {
		case dns.TypeA:
			ar := r.(*dns.A)
			if !ar.A.Equal(expected) {
				t.Errorf("Test %d query %d: expected top IP %s but got %s", i, j, expectedTopIP, ar.A)
			}
			return
		case dns.TypeAAAA:
			ar := r.(*dns.AAAA)
			if !ar.AAAA.Equal(expected) {
				t.Errorf("Test %d query %d: expected top IP %s but got %s", i, j, expectedTopIP, ar.AAAA)
			}
			return
		}
	}
}
