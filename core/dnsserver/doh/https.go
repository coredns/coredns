package doh

import (
	"io/ioutil"
	"net/http"

	"github.com/miekg/dns"
)

// MimeType is the DoH mimetype that should be used.
const MimeType = "application/dns-udpwireformat"

// RequestToMsg extra the dns message from the request body.
func RequestToMsg(req *http.Request) (*dns.Msg, error) {
	defer req.Body.Close()

	buf, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, nil

	}
	m := new(dns.Msg)
	err = m.Unpack(buf)
	return m, err
}
