package doh

import (
	"errors"
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

			req, err := NewRequest(test.method, test.url, "example.org", m)
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

func TestDoHGETRejectsJSONAPI(t *testing.T) {
	tests := map[string]struct {
		target string
		accept string
	}{
		"name and type parameters": {target: "https://example.org" + Path + "?name=example.org&type=A"},
		"accept header only":       {target: "https://example.org" + Path, accept: "application/dns-json"},
		"parameters and accept":    {target: "https://example.org" + Path + "?name=example.org&type=A", accept: "application/dns-json"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, test.target, nil)
			if err != nil {
				t.Fatalf("failed to build request: %v", err)
			}
			if test.accept != "" {
				req.Header.Set("Accept", test.accept)
			}

			_, err = RequestToMsg(req)
			if !errors.Is(err, ErrJSONNotSupported) {
				t.Fatalf("expected ErrJSONNotSupported, got %v", err)
			}
		})
	}
}

func TestDoHGETMissingDNSParameter(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.org"+Path, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	_, err = RequestToMsg(req)
	if err == nil {
		t.Fatal("expected GET request without 'dns' query parameter to be rejected")
	}
	if errors.Is(err, ErrJSONNotSupported) {
		t.Fatalf("expected a generic error for a request without JSON API markers, got %v", err)
	}
	if err.Error() != "no 'dns' query parameter found" {
		t.Fatalf("expected %q, got %v", "no 'dns' query parameter found", err)
	}
}

func TestDoHGETRejectsOversizedDNSQuery(t *testing.T) {
	// Exceeding max size 65536
	raw := make([]byte, 65536+1)
	b64 := b64Enc.EncodeToString(raw)

	req, err := http.NewRequest(
		http.MethodGet,
		"https://example.org"+Path+"?dns="+b64,
		nil,
	)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	_, err = RequestToMsg(req)
	if err == nil {
		t.Fatalf("expected oversized GET dns query to be rejected")
	}
	if err.Error() != "dns query too large" {
		t.Fatalf("expected %q, got %v", "dns query too large", err)
	}
}
