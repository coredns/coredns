package dnsserver

import (
	"io/ioutil"
	"net/http"

	"github.com/miekg/dns"
)

// mimeTypeDOH is the DoH mimetype that should be used.
const mimeTypeDOH = "application/dns-message"

// postRequestToMsg extracts the dns message from the request body.
func postRequestToMsg(req *http.Request) (*dns.Msg, error) {
	defer req.Body.Close()

	buf, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, nil

	}
	m := new(dns.Msg)
	err = m.Unpack(buf)
	return m, err
}
