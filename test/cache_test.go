package test

import (
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/test"

	"github.com/google/go-cmp/cmp"
	"github.com/miekg/dns"
	"github.com/prometheus/common/model"
)

func TestLookupCache(t *testing.T) {
	// Start auth. CoreDNS holding the auth zone.
	name, rm, err := test.TempFile(".", exampleOrg)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	corefile := `example.org:0 {
       file ` + name + `
}
`
	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	// Start caching forward CoreDNS that we want to test.
	corefile = `example.org:0 {
	forward . ` + udp + `
	cache 10
}
`
	i, udp, _, err = CoreDNSServerAndPorts(corefile)
	defer i.Stop()
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	t.Run("Long TTL", func(t *testing.T) {
		testCase(t, "example.org.", udp, 2, 10)
	})

	t.Run("Short TTL", func(t *testing.T) {
		testCase(t, "short.example.org.", udp, 1, 5)
	})

}

func testCase(t *testing.T, name, addr string, expectAnsLen int, expectTTL uint32) {
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeA)
	resp, err := dns.Exchange(m, addr)
	if err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}

	if len(resp.Answer) != expectAnsLen {
		t.Fatalf("Expected %v RR in the answer section, got %v.", expectAnsLen, len(resp.Answer))
	}

	ttl := resp.Answer[0].Header().Ttl
	if ttl != expectTTL {
		t.Errorf("Expected TTL to be %d, got %d", expectTTL, ttl)
	}
}

func TestCacheMetrics(t *testing.T) {
	cacheSize := "coredns_cache_size"
	corefileWithMetrics := `
	.:0 {
		prometheus localhost:0
		cache
		forward . 8.8.8.8 {
           force_tcp
		}
	}`
	inst, _, tcp, err := CoreDNSServerAndPorts(corefileWithMetrics)
	defer inst.Stop()
	if err != nil {
		if strings.Contains(err.Error(), inUse) {
			return
		}
		t.Errorf("Could not get service instance: %s", err)
	}
	// send a query and check we can scrap corresponding metrics
	cl := dns.Client{Net: "tcp"}
	m := new(dns.Msg)
	m.SetQuestion("www.example.org.", dns.TypeA)

	if _, _, err := cl.Exchange(m, tcp); err != nil {
		t.Fatalf("Could not send message: %s", err)
	}
	// coredns cache size
	cacheDenialSizeSamples := map[model.LabelName]model.LabelValue{
		model.MetricNameLabel: model.LabelValue(cacheSize),
		"server":              "dns://:0",
		"type":                "denial",
		"Value":               "0",
	}
	cacheSuccessSizeSamples := map[model.LabelName]model.LabelValue{
		model.MetricNameLabel: model.LabelValue(cacheSize),
		"server":              "dns://:0",
		"type":                "success",
		"Value":               "1",
	}
	expect := test.DnsMetrics(
		map[string]model.Samples{
			cacheSize: model.Samples{test.NewCacheSample(model.LabelValue(cacheSize), cacheDenialSizeSamples), test.NewCacheSample(model.LabelValue(cacheSize), cacheSuccessSizeSamples)},
		},
	)
	actual, err := test.GetDnsMetrics(map[string]interface{}{
		cacheSize: nil,
	}, metrics.ListenAddr)
	if err != nil {
		t.Errorf("Could not get dns metrics: %v", err)
	}
	// compare to expected
	if diff := cmp.Diff(expect, actual); diff != "" {
		t.Errorf("Unexpected result diff (-want, +got): %s", diff)
	}
}
