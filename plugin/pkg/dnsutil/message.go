package dnsutil

import (
	"encoding/binary"
	"errors"

	"github.com/miekg/dns"
)

var errRequestRejected = errors.New("dns request rejected")

// UnpackRequest unpacks a request after applying the default miekg/dns request policy.
func UnpackRequest(msg []byte) (*dns.Msg, error) {
	return UnpackRequestWithAcceptFunc(msg, nil)
}

// UnpackRequestWithAcceptFunc unpacks a request after applying accept's
// request policy instead of the default one -- for a caller (like the
// HTTPS/HTTP3 transports in core/dnsserver) that needs to honor a
// server's own opted-in extra opcodes (Config.AllowOpcode) the same way
// the plain UDP/TCP/TLS transports already do via their dns.Server's
// MsgAcceptFunc, rather than always rejecting anything but an ordinary
// query. A nil accept falls back to dns.DefaultMsgAcceptFunc, exactly
// matching UnpackRequest's own, original behavior.
func UnpackRequestWithAcceptFunc(msg []byte, accept dns.MsgAcceptFunc) (*dns.Msg, error) {
	if accept == nil {
		accept = dns.DefaultMsgAcceptFunc
	}
	var header dns.Header
	if _, err := binary.Decode(msg, binary.BigEndian, &header); err != nil {
		return nil, dns.ErrBuf
	}
	if accept(header) != dns.MsgAccept {
		return nil, errRequestRejected
	}

	request := new(dns.Msg)
	return request, request.Unpack(msg)
}
