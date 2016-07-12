// +build k8sIntegration

package test

import (
	"io/ioutil"
	//	"fmt"
	"log"
	"net/http"
	"testing"

	"github.com/miekg/dns"
)

// Test data for A records
var testdataLookupA = []struct {
	Query            string
	TotalAnswerCount int
	ARecordCount     int
}{
	// Matching queries
	{"mynginx.demo.coredns.local.", 1, 1},                     // One A record, should exist
	// Failure queries
	{"mynginx.test.coredns.local.", 0, 0},                     // One A record, is not exposed
	{"someservicethatdoesnotexist.demo.coredns.local.", 0, 0}, // Record does not exist
	// Namespace wildcards
	{"mynginx.*.coredns.local.", 1, 1},                        // One A record, via wildcard namespace
	{"mynginx.any.coredns.local.", 1, 1},                      // One A record, via wildcard namespace
	{"someservicethatdoesnotexist.*.coredns.local.", 0, 0},    // Record does not exist with wildcard for namespace
//	{"*.demo.coredns.local.", 1, 1},                           // One A record, via wildcard
//	{"*.test.coredns.local.", 0, 0},                           // One A record, via wildcard that is not exposed
}

// checkKubernetesRunning performs a basic
func checkKubernetesRunning() bool {
	_, err := http.Get("http://localhost:8080/api/v1")
	return err == nil
}



func TestK8sIntegration(t *testing.T) {
	t.Log("   === RUN testLookupA")
	testLookupA(t)
}


func testLookupA(t *testing.T) {

	if ! checkKubernetesRunning() {
		t.Skip("Skipping Kubernetes Integration tests. Kubernetes is not running")
	}

	coreFile := `
.:1053 {
    kubernetes coredns.local {
		endpoint http://localhost:8080
		namespaces demo
    }
`
	server, _, udp, err := Server(t, coreFile)
	if err != nil {
		t.Fatal("Could not get server: %s", err)
	}
	defer server.Stop()

	log.SetOutput(ioutil.Discard)

	dnsClient := new(dns.Client)
	dnsMessage := new(dns.Msg)

	for _, testData := range testdataLookupA {

		dnsMessage.SetQuestion(testData.Query, dns.TypeA)
		dnsMessage.SetEdns0(4096, true)
		res, _, err := dnsClient.Exchange(dnsMessage, udp)
		if err != nil {
			t.Fatal("Could not send query: %s", err)
		}
		// Count A records in the answer section
		ARecordCount := 0
		for _, a := range res.Answer {
			if a.Header().Rrtype == dns.TypeA {
				ARecordCount++
			}
		}

		if ARecordCount != testData.ARecordCount {
			t.Errorf("Expected '%v' A records in response. Instead got '%v' A records. Test query string: '%v'", testData.ARecordCount, ARecordCount, testData.Query)
		}
		if len(res.Answer) != testData.TotalAnswerCount {
			t.Errorf("Expected '%v' records in answer section. Instead got '%v' records in answer section. Test query string: '%v'", res.Answer, testData.TotalAnswerCount, testData.Query)
		}
	}
}
