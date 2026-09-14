package doh

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/coredns/coredns/plugin/pkg/dnsutil"

	"github.com/miekg/dns"
)

// MimeType is the DoH mimetype that should be used.
const MimeType = "application/dns-message"

// JSONMimeType marks a POST body as a JSONWireEnvelope rather than raw
// DNS wire bytes -- not an RFC 8484 content type (DoH itself defines
// none for JSON), but a minimal convention this package also recognizes
// so a caller with no easy way to send a raw binary POST body (some
// HTTP client libraries, browser fetch() call sites, etc.) still has a
// byte-exact way to carry an arbitrary DNS message, opcode included, in
// JSON. See JSONWireEnvelope's own doc comment for why this wraps raw
// bytes rather than representing the message's fields structurally.
const JSONMimeType = "application/dns-message+json"

// Path is the URL path that should be used.
const Path = "/dns-query"

// JSONWireEnvelope carries a DNS message's exact wire bytes as base64
// inside a small JSON object, for a POST body with Content-Type
// JSONMimeType (or the generic "application/json"). This is
// deliberately NOT a structural (RFC 8427) JSON representation of the
// message's individual fields: some DNS message authentication schemes
// (e.g. SIG(0), RFC 2931) sign the literal wire bytes a sender
// transmitted, and there is no generally lossless mapping back from
// parsed JSON fields to that exact byte sequence (canonical name
// compression, casing, and section ordering are all wire-level details
// a structural JSON encoding does not have to preserve). Wrapping the
// same raw bytes in base64 has no such problem -- decoding it recovers
// them exactly, so any authentication computed over the original wire
// form still verifies.
type JSONWireEnvelope struct {
	// Wire is the message's raw wire bytes, base64-encoded (standard
	// encoding, i.e. encoding/base64.StdEncoding).
	Wire string `json:"wire"`
}

// NewRequest returns a new DoH request given a HTTP method, URL and dns.Msg.
//
// The URL should not have a path, so please exclude /dns-query. The URL will
// be prefixed with https:// by default, unless it's already prefixed with
// either http:// or https://.
func NewRequest(method, url, host string, m *dns.Msg) (*http.Request, error) {
	return NewRequestWithContext(context.Background(), method, url, host, m)
}

func NewRequestWithContext(ctx context.Context, method, url, host string, m *dns.Msg) (*http.Request, error) {
	buf, err := m.Pack()
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	switch method {
	case http.MethodGet:
		b64 := base64.RawURLEncoding.EncodeToString(buf)

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			fmt.Sprintf("%s%s?dns=%s", url, Path, b64),
			nil,
		)
		if err != nil {
			return req, err
		}

		req.Header.Set("Content-Type", MimeType)
		req.Header.Set("Accept", MimeType)
		req.Host = host
		return req, nil

	case http.MethodPost:
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			fmt.Sprintf("%s%s", url, Path),
			bytes.NewReader(buf),
		)
		if err != nil {
			return req, err
		}

		req.Header.Set("Content-Type", MimeType)
		req.Header.Set("Accept", MimeType)
		req.Host = host
		return req, nil

	default:
		return nil, fmt.Errorf("method not allowed: %s", method)
	}
}

// ResponseToMsg converts a http.Response to a dns message.
func ResponseToMsg(resp *http.Response) (*dns.Msg, error) {
	defer resp.Body.Close()

	return toMsg(resp.Body)
}

// RequestToMsg converts a http.Request to a dns message.
func RequestToMsg(req *http.Request) (*dns.Msg, error) {
	msg, _, err := RequestToMsgWire(req)
	return msg, err
}

// RequestToMsgWire converts a http.Request to a dns message and returns the
// original DNS wire bytes from the request. Applies miekg/dns's default
// request policy (ordinary queries and notifies only) -- see
// RequestToMsgWireWithAccept to also allow whatever extra opcodes the
// server's own configuration has opted into (Config.AllowOpcode).
func RequestToMsgWire(req *http.Request) (*dns.Msg, []byte, error) {
	return RequestToMsgWireWithAccept(req, nil)
}

// RequestToMsgWireWithAccept is RequestToMsgWire, but unpacks the
// message with accept's request policy instead of always requiring the
// default one -- for a caller (core/dnsserver's HTTPS/HTTP3 transports)
// that needs a plugin's opted-in extra opcodes (e.g. RFC 2136 dynamic
// UPDATE) to reach it consistently across every transport its Config
// serves, not just plain UDP/TCP/TLS. A nil accept is exactly
// RequestToMsgWire's own, original behavior.
func RequestToMsgWireWithAccept(req *http.Request, accept dns.MsgAcceptFunc) (*dns.Msg, []byte, error) {
	switch req.Method {
	case http.MethodGet:
		return requestToMsgGet(req, accept)

	case http.MethodPost:
		return requestToMsgPost(req, accept)

	default:
		return nil, nil, fmt.Errorf("method not allowed: %s", req.Method)
	}
}

// requestToMsgPost extracts the dns message from the request body: for
// Content-Type JSONMimeType or "application/json", the body is a
// JSONWireEnvelope wrapping the wire bytes in base64; for anything else
// (including the RFC 8484 default, MimeType, and an unset Content-Type),
// the body is exactly the wire bytes themselves.
func requestToMsgPost(req *http.Request, accept dns.MsgAcceptFunc) (*dns.Msg, []byte, error) {
	defer req.Body.Close()
	buf, err := io.ReadAll(http.MaxBytesReader(nil, req.Body, maxDNSQuerySize))
	if err != nil {
		return nil, nil, err
	}
	if isJSONContentType(req.Header.Get("Content-Type")) {
		buf, err = decodeJSONWireEnvelope(buf)
		if err != nil {
			return nil, nil, err
		}
	}
	m, err := dnsutil.UnpackRequestWithAcceptFunc(buf, accept)
	return m, buf, err
}

// isJSONContentType reports whether ct names a JSON-envelope POST body
// (JSONMimeType, or the generic "application/json" for a caller that has
// no easy way to set a bespoke content type).
func isJSONContentType(ct string) bool {
	if ct == "" {
		return false
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return mt == JSONMimeType || mt == "application/json"
}

// decodeJSONWireEnvelope parses buf as a JSONWireEnvelope and returns
// the raw wire bytes it carries.
func decodeJSONWireEnvelope(buf []byte) ([]byte, error) {
	var env JSONWireEnvelope
	if err := json.Unmarshal(buf, &env); err != nil {
		return nil, fmt.Errorf("decoding JSON wire envelope: %w", err)
	}
	wire, err := base64.StdEncoding.DecodeString(env.Wire)
	if err != nil {
		return nil, fmt.Errorf("decoding JSON wire envelope's base64 wire field: %w", err)
	}
	return wire, nil
}

const maxDNSQuerySize = 65536
const maxBase64Len = (maxDNSQuerySize*8 + 5) / 6

// requestToMsgGet extract the dns message from the GET request.
func requestToMsgGet(req *http.Request, accept dns.MsgAcceptFunc) (*dns.Msg, []byte, error) {
	values := req.URL.Query()
	b64, ok := values["dns"]
	if !ok {
		return nil, nil, fmt.Errorf("no 'dns' query parameter found")
	}
	if len(b64) != 1 {
		return nil, nil, fmt.Errorf("multiple 'dns' query values found")
	}
	if len(b64[0]) > maxBase64Len {
		return nil, nil, fmt.Errorf("dns query too large")
	}
	return base64ToMsgWireWithAccept(b64[0], accept)
}

func toMsg(r io.ReadCloser) (*dns.Msg, error) {
	m, _, err := toMsgWire(r)
	return m, err
}

func toMsgWire(r io.ReadCloser) (*dns.Msg, []byte, error) {
	buf, err := io.ReadAll(http.MaxBytesReader(nil, r, maxDNSQuerySize))
	if err != nil {
		return nil, nil, err
	}
	m := new(dns.Msg)
	err = m.Unpack(buf)
	return m, buf, err
}

func base64ToMsgWireWithAccept(b64 string, accept dns.MsgAcceptFunc) (*dns.Msg, []byte, error) {
	buf, err := b64Enc.DecodeString(b64)
	if err != nil {
		return nil, nil, err
	}

	m, err := dnsutil.UnpackRequestWithAcceptFunc(buf, accept)
	return m, buf, err
}

var b64Enc = base64.RawURLEncoding
