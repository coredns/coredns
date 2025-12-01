package cache

import (
	"fmt"
	"testing"
	"time"

	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input            string
		shouldErr        bool
		expectedNcap     int
		expectedPcap     int
		expectedNttl     time.Duration
		expectedMinNttl  time.Duration
		expectedPttl     time.Duration
		expectedMinPttl  time.Duration
		expectedPrefetch int
	}{
		{`cache`, false, defaultCap, defaultCap, maxNTTL, minNTTL, maxTTL, minTTL, 0},
		{`cache {}`, false, defaultCap, defaultCap, maxNTTL, minNTTL, maxTTL, minTTL, 0},
		{`cache example.nl {
				success 10
			}`, false, defaultCap, 10, maxNTTL, minNTTL, maxTTL, minTTL, 0},
		{`cache example.nl {
				success 10 1800 30
			}`, false, defaultCap, 10, maxNTTL, minNTTL, 1800 * time.Second, 30 * time.Second, 0},
		{`cache example.nl {
				success 10
				denial 10 15
			}`, false, 10, 10, 15 * time.Second, minNTTL, maxTTL, minTTL, 0},
		{`cache example.nl {
				success 10
				denial 10 15 2
			}`, false, 10, 10, 15 * time.Second, 2 * time.Second, maxTTL, minTTL, 0},
		{`cache 25 example.nl {
				success 10
				denial 10 15
			}`, false, 10, 10, 15 * time.Second, minNTTL, 25 * time.Second, minTTL, 0},
		{`cache 25 example.nl {
				success 10
				denial 10 15 5
			}`, false, 10, 10, 15 * time.Second, 5 * time.Second, 25 * time.Second, minTTL, 0},
		{`cache aaa example.nl`, false, defaultCap, defaultCap, maxNTTL, minNTTL, maxTTL, minTTL, 0},
		{`cache	{
				prefetch 10
			}`, false, defaultCap, defaultCap, maxNTTL, minNTTL, maxTTL, minTTL, 10},

		// fails
		{`cache example.nl {
				success
				denial 10 15
			}`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0},
		{`cache example.nl {
				success 15
				denial aaa
			}`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0},
		{`cache example.nl {
				positive 15
				negative aaa
			}`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0},
		{`cache 0 example.nl`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0},
		{`cache -1 example.nl`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0},
		{`cache 1 example.nl {
				positive 0
			}`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0},
		{`cache 1 example.nl {
				positive 0
				prefetch -1
			}`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0},
		{`cache 1 example.nl {
				prefetch 0 blurp
			}`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0},
		{`cache
		  cache`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		ca, err := cacheParse(c)
		if test.shouldErr && err == nil {
			t.Errorf("Test %v: Expected error but found nil", i)
			continue
		} else if !test.shouldErr && err != nil {
			t.Errorf("Test %v: Expected no error but found error: %v", i, err)
			continue
		}
		if test.shouldErr && err != nil {
			continue
		}

		if ca.ncap != test.expectedNcap {
			t.Errorf("Test %v: Expected ncap %v but found: %v", i, test.expectedNcap, ca.ncap)
		}
		if ca.pcap != test.expectedPcap {
			t.Errorf("Test %v: Expected pcap %v but found: %v", i, test.expectedPcap, ca.pcap)
		}
		if ca.nttl != test.expectedNttl {
			t.Errorf("Test %v: Expected nttl %v but found: %v", i, test.expectedNttl, ca.nttl)
		}
		if ca.minnttl != test.expectedMinNttl {
			t.Errorf("Test %v: Expected minnttl %v but found: %v", i, test.expectedMinNttl, ca.minnttl)
		}
		if ca.pttl != test.expectedPttl {
			t.Errorf("Test %v: Expected pttl %v but found: %v", i, test.expectedPttl, ca.pttl)
		}
		if ca.minpttl != test.expectedMinPttl {
			t.Errorf("Test %v: Expected minpttl %v but found: %v", i, test.expectedMinPttl, ca.minpttl)
		}
		if ca.prefetch != test.expectedPrefetch {
			t.Errorf("Test %v: Expected prefetch %v but found: %v", i, test.expectedPrefetch, ca.prefetch)
		}
	}
}

func TestServeStale(t *testing.T) {
	tests := []struct {
		input       string
		shouldErr   bool
		staleUpTo   time.Duration
		verifyStale bool
	}{
		{"serve_stale", false, 1 * time.Hour, false},
		{"serve_stale 20m", false, 20 * time.Minute, false},
		{"serve_stale 1h20m", false, 80 * time.Minute, false},
		{"serve_stale 0m", false, 0, false},
		{"serve_stale 0", false, 0, false},
		{"serve_stale 0 verify", false, 0, true},
		{"serve_stale 0 immediate", false, 0, false},
		{"serve_stale 0 VERIFY", false, 0, true},
		// fails
		{"serve_stale 20", true, 0, false},
		{"serve_stale -20m", true, 0, false},
		{"serve_stale aa", true, 0, false},
		{"serve_stale 1m nono", true, 0, false},
		{"serve_stale 0 after nono", true, 0, false},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", fmt.Sprintf("cache {\n%s\n}", test.input))
		ca, err := cacheParse(c)
		if test.shouldErr && err == nil {
			t.Errorf("Test %v: Expected error but found nil", i)
			continue
		} else if !test.shouldErr && err != nil {
			t.Errorf("Test %v: Expected no error but found error: %v", i, err)
			continue
		}
		if test.shouldErr && err != nil {
			continue
		}
		if ca.staleUpTo != test.staleUpTo {
			t.Errorf("Test %v: Expected stale %v but found: %v", i, test.staleUpTo, ca.staleUpTo)
		}
	}
}

func TestServfail(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		failttl   time.Duration
	}{
		{"servfail 1s", false, 1 * time.Second},
		{"servfail 5m", false, 5 * time.Minute},
		{"servfail 0s", false, 0},
		{"servfail 0", false, 0},
		// fails
		{"servfail", true, minNTTL},
		{"servfail 6m", true, minNTTL},
		{"servfail 20", true, minNTTL},
		{"servfail -1s", true, minNTTL},
		{"servfail aa", true, minNTTL},
		{"servfail 1m invalid", true, minNTTL},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", fmt.Sprintf("cache {\n%s\n}", test.input))
		ca, err := cacheParse(c)
		if test.shouldErr && err == nil {
			t.Errorf("Test %v: Expected error but found nil", i)
			continue
		} else if !test.shouldErr && err != nil {
			t.Errorf("Test %v: Expected no error but found error: %v", i, err)
			continue
		}
		if test.shouldErr && err != nil {
			continue
		}
		if ca.failttl != test.failttl {
			t.Errorf("Test %v: Expected stale %v but found: %v", i, test.failttl, ca.staleUpTo)
		}
	}
}

func TestDisable(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		nexcept   []string
		pexcept   []string
	}{
		// positive
		{"disable denial example.com example.org", false, []string{"example.com.", "example.org."}, nil},
		{"disable success example.com example.org", false, nil, []string{"example.com.", "example.org."}},
		{"disable denial", false, []string{"."}, nil},
		{"disable success", false, nil, []string{"."}},
		{"disable denial example.com example.org\ndisable success example.com example.org", false,
			[]string{"example.com.", "example.org."}, []string{"example.com.", "example.org."}},
		// negative
		{"disable invalid example.com example.org", true, nil, nil},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", fmt.Sprintf("cache {\n%s\n}", test.input))
		ca, err := cacheParse(c)
		if test.shouldErr && err == nil {
			t.Errorf("Test %v: Expected error but found nil", i)
			continue
		} else if !test.shouldErr && err != nil {
			t.Errorf("Test %v: Expected no error but found error: %v", i, err)
			continue
		}
		if test.shouldErr {
			continue
		}
		if fmt.Sprintf("%v", test.nexcept) != fmt.Sprintf("%v", ca.nexcept) {
			t.Errorf("Test %v: Expected %v but got: %v", i, test.nexcept, ca.nexcept)
		}
		if fmt.Sprintf("%v", test.pexcept) != fmt.Sprintf("%v", ca.pexcept) {
			t.Errorf("Test %v: Expected %v but got: %v", i, test.pexcept, ca.pexcept)
		}
	}
}

func TestKeepttl(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
	}{
		// positive
		{"keepttl", false},
		// negative
		{"keepttl arg1", true},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", fmt.Sprintf("cache {\n%s\n}", test.input))
		ca, err := cacheParse(c)
		if test.shouldErr && err == nil {
			t.Errorf("Test %v: Expected error but found nil", i)
			continue
		} else if !test.shouldErr && err != nil {
			t.Errorf("Test %v: Expected no error but found error: %v", i, err)
			continue
		}
		if test.shouldErr {
			continue
		}
		if !ca.keepttl {
			t.Errorf("Test %v: Expected keepttl enabled but disabled", i)
		}
	}
}
