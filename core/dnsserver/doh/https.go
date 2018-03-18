package doh

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/miekg/dns"
)

const mime = "application/dns-udpwireformat"

// MsgToRequest wraps m in a http POST request according to the DNS over HTTPS Spec.
func MsgToRequest(m *dns.Msg, url string) (*http.Request, error) {
	out, err := m.Pack()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(out))
	req.Header.Set("content-type", mime)
	req.Header.Set("accept", mime)

	return req, nil
}

// ResponseToMsg extracts a dns.Msg from the response body. The resp.Body is closed
// after this operation.
func ResponseToMsg(resp *http.Response) (*dns.Msg, error) {
	defer resp.Body.Close()
	return msgFromReader(resp.Body)
}

// RequestToMsg extra the dns message from the request body.
func RequestToMsg(req *http.Request) (*dns.Msg, error) {
	defer req.Body.Close()
	return msgFromReader(req.Body)
}

func msgFromReader(r io.Reader) (*dns.Msg, error) {
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, nil

	}
	m := new(dns.Msg)
	err = m.Unpack(buf)
	return m, err
}
