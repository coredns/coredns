package doh

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/coredns/coredns/plugin/pkg/dnsutil"

	"github.com/miekg/dns"
)

// MimeType is the DoH mimetype that should be used.
const MimeType = "application/dns-message"

// mimeTypeJSON is the mimetype of the non-standard JSON DNS API offered by
// some public resolvers. CoreDNS does not implement it.
const mimeTypeJSON = "application/dns-json"

// Path is the URL path that should be used.
const Path = "/dns-query"

// ErrJSONNotSupported is returned for GET requests that use the non-standard
// JSON DNS API (application/dns-json) instead of RFC 8484. The error text is
// a fixed string that reveals no request or server internals, so it is safe
// to return to the client.
var ErrJSONNotSupported = errors.New("the JSON API (" + mimeTypeJSON + ") is not supported; use RFC 8484 (" + MimeType + ")")

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
// original DNS wire bytes from the request.
func RequestToMsgWire(req *http.Request) (*dns.Msg, []byte, error) {
	switch req.Method {
	case http.MethodGet:
		return requestToMsgGet(req)

	case http.MethodPost:
		return requestToMsgPost(req)

	default:
		return nil, nil, fmt.Errorf("method not allowed: %s", req.Method)
	}
}

// requestToMsgPost extracts the dns message from the request body.
func requestToMsgPost(req *http.Request) (*dns.Msg, []byte, error) {
	defer req.Body.Close()
	buf, err := io.ReadAll(http.MaxBytesReader(nil, req.Body, maxDNSQuerySize))
	if err != nil {
		return nil, nil, err
	}
	m, err := dnsutil.UnpackRequest(buf)
	return m, buf, err
}

const maxDNSQuerySize = 65536
const maxBase64Len = (maxDNSQuerySize*8 + 5) / 6

// requestToMsgGet extract the dns message from the GET request.
func requestToMsgGet(req *http.Request) (*dns.Msg, []byte, error) {
	values := req.URL.Query()
	b64, ok := values["dns"]
	if !ok {
		// A 'name' parameter or an application/dns-json Accept header means
		// the client is using the JSON DNS API as implemented by e.g. the
		// Google and Cloudflare public resolvers, see
		// https://developers.google.com/speed/public-dns/docs/doh/json.
		// Return a specific error so the server can tell the client why the
		// request is rejected.
		if values.Has("name") || strings.Contains(req.Header.Get("Accept"), mimeTypeJSON) {
			return nil, nil, ErrJSONNotSupported
		}
		return nil, nil, fmt.Errorf("no 'dns' query parameter found")
	}
	if len(b64) != 1 {
		return nil, nil, fmt.Errorf("multiple 'dns' query values found")
	}
	if len(b64[0]) > maxBase64Len {
		return nil, nil, fmt.Errorf("dns query too large")
	}
	return base64ToMsgWire(b64[0])
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

func base64ToMsgWire(b64 string) (*dns.Msg, []byte, error) {
	buf, err := b64Enc.DecodeString(b64)
	if err != nil {
		return nil, nil, err
	}

	m, err := dnsutil.UnpackRequest(buf)
	return m, buf, err
}

var b64Enc = base64.RawURLEncoding
