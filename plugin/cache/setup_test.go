package cache

import (
	"testing"
	"time"

	"github.com/mholt/caddy"
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
		expectedZones    []string
	}{
		{`cache 10.0.0.0/31`, false, defaultCap, defaultCap, maxNTTL, minNTTL, maxTTL, minTTL, 0, []string{"0.0.0.10.in-addr.arpa.", "1.0.0.10.in-addr.arpa."}},
		{`cache`, false, defaultCap, defaultCap, maxNTTL, minNTTL, maxTTL, minTTL, 0, []string{}},
		{`cache {}`, false, defaultCap, defaultCap, maxNTTL, minNTTL, maxTTL, minTTL, 0, []string{"{}."}},
		{`cache example.nl {
				success 10
			}`, false, defaultCap, 10, maxNTTL, minNTTL, maxTTL, minTTL, 0, []string{"example.nl."}},
		{`cache example.nl {
				success 10 1800 30
			}`, false, defaultCap, 10, maxNTTL, minNTTL, 1800 * time.Second, 30 * time.Second, 0, []string{"example.nl."}},
		{`cache example.nl {
				success 10
				denial 10 15
			}`, false, 10, 10, 15 * time.Second, minNTTL, maxTTL, minTTL, 0, []string{"example.nl."}},
		{`cache example.nl {
				success 10
				denial 10 15 2
			}`, false, 10, 10, 15 * time.Second, 2 * time.Second, maxTTL, minTTL, 0, []string{"example.nl."}},
		{`cache 25 example.nl {
				success 10
				denial 10 15
			}`, false, 10, 10, 15 * time.Second, minNTTL, 25 * time.Second, minTTL, 0, []string{"example.nl."}},
		{`cache 25 example.nl {
				success 10
				denial 10 15 5
			}`, false, 10, 10, 15 * time.Second, 5 * time.Second, 25 * time.Second, minTTL, 0, []string{"example.nl."}},
		{`cache aaa example.nl`, false, defaultCap, defaultCap, maxNTTL, minNTTL, maxTTL, minTTL, 0, []string{"aaa.", "example.nl."}},
		{`cache	{
				prefetch 10
			}`, false, defaultCap, defaultCap, maxNTTL, minNTTL, maxTTL, minTTL, 10, []string{}},

		//// fails
		{`cache example.nl {
				success
				denial 10 15
			}`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0, []string{"example.nl."}},
		{`cache example.nl {
				success 15
				denial aaa
			}`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0, []string{"example.nl."}},
		{`cache example.nl {
				positive 15
				negative aaa
			}`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0, []string{"example.nl."}},
		{`cache 0 example.nl`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0, []string{"example.nl."}},
		{`cache -1 example.nl`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0, []string{"example.nl."}},
		{`cache 1 example.nl {
				positive 0
			}`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0, []string{"example.nl."}},
		{`cache 1 example.nl {
				positive 0
				prefetch -1
			}`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0, []string{"example.nl."}},
		{`cache 1 example.nl {
				prefetch 0 blurp
			}`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0, []string{"example.nl."}},
		{`cache
		 cache`, true, defaultCap, defaultCap, maxTTL, minNTTL, maxTTL, minTTL, 0, []string{}},
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
		if len(ca.Zones) != len(test.expectedZones) {
			t.Errorf("Test %v: Expected %d zones but got %d", i, len(test.expectedZones), len(ca.Zones))
		}
		for j, zone := range ca.Zones {
			if test.expectedZones[j] != zone {
				t.Errorf("Test %v: expected zone %q for input %s, got: %q", i, test.expectedZones[j], test.input, zone)
			}
		}
	}
}
