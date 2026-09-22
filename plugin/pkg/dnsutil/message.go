package dnsutil

import (
	"encoding/binary"
	"errors"

	"github.com/miekg/dns"
)

var errRequestRejected = errors.New("dns request rejected")

// UnpackRequest unpacks a request after applying the default miekg/dns request policy.
func UnpackRequest(msg []byte) (*dns.Msg, error) {
	var header dns.Header
	if _, err := binary.Decode(msg, binary.BigEndian, &header); err != nil {
		return nil, dns.ErrBuf
	}
	if dns.DefaultMsgAcceptFunc(header) != dns.MsgAccept {
		return nil, errRequestRejected
	}

	request := new(dns.Msg)
	return request, request.Unpack(msg)
}
