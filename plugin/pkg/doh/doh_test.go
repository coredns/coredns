package doh

import (
	"net/http"
	"testing"

	"github.com/miekg/dns"
)

func TestDoH(t *testing.T) {
	tests := map[string]struct {
		method string
		url    string
	}{
		"POST request over HTTPS":       {method: http.MethodPost, url: "https://example.org:443"},
		"POST request over HTTP":        {method: http.MethodPost, url: "http://example.org:443"},
		"POST request without protocol": {method: http.MethodPost, url: "example.org:443"},
		"GET request over HTTPS":        {method: http.MethodGet, url: "https://example.org:443"},
		"GET request over HTTP":         {method: http.MethodGet, url: "http://example.org"},
		"GET request without protocol":  {method: http.MethodGet, url: "example.org:443"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion("example.org.", dns.TypeDNSKEY)

			req, err := NewRequest(test.method, test.url, m)
			if err != nil {
				t.Errorf("Failure to make request: %s", err)
			}

			m, err = RequestToMsg(req)
			if err != nil {
				t.Fatalf("Failure to get message from request: %s", err)
			}

			if x := m.Question[0].Name; x != "example.org." {
				t.Errorf("Qname expected %s, got %s", "example.org.", x)
			}
			if x := m.Question[0].Qtype; x != dns.TypeDNSKEY {
				t.Errorf("Qname expected %d, got %d", x, dns.TypeDNSKEY)
			}
		})
	}
}
