package doh

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
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

func TestRequestToMsgWireRejectsUpdateByDefault(t *testing.T) {
	m := new(dns.Msg)
	m.SetUpdate("example.org.")
	wire, err := m.Pack()
	if err != nil {
		t.Fatalf("packing: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "https://example.org"+Path, bytes.NewReader(wire))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", MimeType)

	if _, _, err := RequestToMsgWire(req); err == nil {
		t.Fatalf("expected an UPDATE-opcode message to be rejected without an accept func opting it in")
	}
}

// allowUpdateAccept mimics core/dnsserver's Server.acceptMessage for a
// config that has called Config.AllowOpcode(dns.OpcodeUpdate): the
// default policy, except UPDATE (with exactly one question) is also let
// through.
func allowUpdateAccept(h dns.Header) dns.MsgAcceptAction {
	action := dns.DefaultMsgAcceptFunc(h)
	if action != dns.MsgRejectNotImplemented {
		return action
	}
	opcode := int(h.Bits>>11) & 0xF
	if opcode == dns.OpcodeUpdate && h.Qdcount == 1 {
		return dns.MsgAccept
	}
	return action
}

func TestRequestToMsgWireWithAcceptAllowsOptedInOpcode(t *testing.T) {
	m := new(dns.Msg)
	m.SetUpdate("example.org.")
	wire, err := m.Pack()
	if err != nil {
		t.Fatalf("packing: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "https://example.org"+Path, bytes.NewReader(wire))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", MimeType)

	msg, raw, err := RequestToMsgWireWithAccept(req, allowUpdateAccept)
	if err != nil {
		t.Fatalf("expected an opted-in UPDATE opcode to be accepted, got: %v", err)
	}
	if msg.Opcode != dns.OpcodeUpdate {
		t.Fatalf("expected the unpacked message to keep its UPDATE opcode, got %d", msg.Opcode)
	}
	if !bytes.Equal(raw, wire) {
		t.Fatalf("expected the returned raw bytes to exactly match the original wire bytes")
	}
}

func TestRequestToMsgWireWithAcceptJSONEnvelope(t *testing.T) {
	m := new(dns.Msg)
	m.SetUpdate("example.org.")
	wire, err := m.Pack()
	if err != nil {
		t.Fatalf("packing: %v", err)
	}

	envelope, err := json.Marshal(JSONWireEnvelope{Wire: base64.StdEncoding.EncodeToString(wire)})
	if err != nil {
		t.Fatalf("marshaling envelope: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "https://example.org"+Path, bytes.NewReader(envelope))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", JSONMimeType)

	msg, raw, err := RequestToMsgWireWithAccept(req, allowUpdateAccept)
	if err != nil {
		t.Fatalf("expected a valid JSON wire envelope to be accepted, got: %v", err)
	}
	if msg.Opcode != dns.OpcodeUpdate {
		t.Fatalf("expected the unpacked message to keep its UPDATE opcode, got %d", msg.Opcode)
	}
	// The returned raw bytes must be the exact original wire bytes -- not
	// the JSON envelope bytes -- since a caller (e.g. SIG(0) verification)
	// needs to hash/verify against exactly what was signed.
	if !bytes.Equal(raw, wire) {
		t.Fatalf("expected the returned raw bytes to be the decoded wire bytes, not the JSON envelope")
	}
}

func TestRequestToMsgWireWithAcceptJSONEnvelopeGenericContentType(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	wire, err := m.Pack()
	if err != nil {
		t.Fatalf("packing: %v", err)
	}
	envelope, err := json.Marshal(JSONWireEnvelope{Wire: base64.StdEncoding.EncodeToString(wire)})
	if err != nil {
		t.Fatalf("marshaling envelope: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "https://example.org"+Path, bytes.NewReader(envelope))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	// The generic "application/json" content type (for a caller with no
	// easy way to set a bespoke one) must also be recognized, with an
	// optional charset parameter tolerated.
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	if _, raw, err := RequestToMsgWireWithAccept(req, nil); err != nil {
		t.Fatalf("expected generic application/json to be recognized, got: %v", err)
	} else if !bytes.Equal(raw, wire) {
		t.Fatalf("expected the decoded wire bytes back")
	}
}

func TestRequestToMsgWireWithAcceptJSONEnvelopeRejectsMalformedJSON(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://example.org"+Path, strings.NewReader("not json"))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", JSONMimeType)

	if _, _, err := RequestToMsgWireWithAccept(req, nil); err == nil {
		t.Fatalf("expected malformed JSON to be rejected")
	}
}

func TestRequestToMsgWireWithAcceptJSONEnvelopeRejectsBadBase64(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://example.org"+Path, strings.NewReader(`{"wire":"not-base64!!"}`))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", JSONMimeType)

	if _, _, err := RequestToMsgWireWithAccept(req, nil); err == nil {
		t.Fatalf("expected invalid base64 in the wire field to be rejected")
	}
}

func TestPlainDNSMessageContentTypeIsNeverTreatedAsJSON(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	wire, err := m.Pack()
	if err != nil {
		t.Fatalf("packing: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "https://example.org"+Path, bytes.NewReader(wire))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", MimeType)

	msg, raw, err := RequestToMsgWireWithAccept(req, nil)
	if err != nil {
		t.Fatalf("expected an ordinary application/dns-message POST to work as before: %v", err)
	}
	if msg.Question[0].Name != "example.org." || !bytes.Equal(raw, wire) {
		t.Fatalf("expected the raw wire body to be used as-is")
	}
}
