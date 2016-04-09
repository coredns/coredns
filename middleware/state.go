package middleware

import (
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// This file contains the state functions available for use in the middlewares.

// State contains some connection state and is useful in middleware.
type State struct {
	Req *dns.Msg
	W   dns.ResponseWriter

	cached bool // true when we already have this information.
	size   int
	do     bool
	opt    *dns.OPT
}

// Now returns the current timestamp in the specified format.
func (s *State) Now(format string) string { return time.Now().Format(format) }

// NowDate returns the current date/time that can be used in other time functions.
func (s *State) NowDate() time.Time { return time.Now() }

// Header gets the header of the request in State.
// TODO(miek)
func (s *State) Header() *dns.RR_Header {
	return nil
}

// IP gets the (remote) IP address of the client making the request.
func (s *State) IP() string {
	ip, _, err := net.SplitHostPort(s.W.RemoteAddr().String())
	if err != nil {
		return s.W.RemoteAddr().String()
	}
	return ip
}

// Post gets the (remote) Port of the client making the request.
func (s *State) Port() (string, error) {
	_, port, err := net.SplitHostPort(s.W.RemoteAddr().String())
	if err != nil {
		return "0", err
	}
	return port, nil
}

// RemoteAddr returns the net.Addr of the client that sent the current request.
func (s *State) RemoteAddr() string {
	return s.W.RemoteAddr().String()
}

// Proto gets the protocol used as the transport. This
// will be udp or tcp.
func (s *State) Proto() string {
	if _, ok := s.W.RemoteAddr().(*net.UDPAddr); ok {
		return "udp"
	}
	if _, ok := s.W.RemoteAddr().(*net.TCPAddr); ok {
		return "tcp"
	}
	return "udp"
}

// Family returns the family of the transport. 1 for IPv4 and 2 for IPv6.
func (s *State) Family() int {
	var a net.IP
	ip := s.W.RemoteAddr()
	if i, ok := ip.(*net.UDPAddr); ok {
		a = i.IP
	}
	if i, ok := ip.(*net.TCPAddr); ok {
		a = i.IP
	}

	if a.To4() != nil {
		return 1
	}
	return 2
}

// Opt returns the OPT record from the request.
func (s *State) Opt() *dns.OPT {
	if s.cached {
		return s.opt
	}
	if o := s.Req.IsEdns0(); o != nil {
		return o
	}
	return nil
}

// Do returns if the request has the DO (DNSSEC OK) bit set.
func (s *State) Do() bool {
	if s.cached {
		return s.do
	}

	s.do = false
	if o := s.Req.IsEdns0(); o != nil {
		s.opt = o
		s.do = o.Do()
	}
	s.cached = true
	return s.do
}

// UDPSize returns if UDP buffer size advertised in the requests OPT record.
// Or when the request was over TCP, we return the maximum allowed size of 64K.
func (s *State) Size() int {
	if s.cached || s.size != 0 {
		return s.size
	}

	if s.Proto() == "tcp" {
		s.size = dns.MaxMsgSize
		if o := s.Req.IsEdns0(); o != nil {
			s.opt = o
			s.do = o.Do()
		}
		s.cached = true
		return dns.MaxMsgSize
	}
	if o := s.Req.IsEdns0(); o != nil {
		s.opt = o
		s.do = o.Do()
		size := o.UDPSize()
		if size < dns.MinMsgSize {
			size = dns.MinMsgSize
		}
		s.size = int(size)
		s.cached = true
		return int(size)
	}
	s.size = dns.MinMsgSize
	s.cached = true
	return dns.MinMsgSize
}

// SizeAndDo returns a ready made OPT record that the reflects the intent from
// state. This can be added to upstream requests that will then hopefully
// return a message that is fits the buffer in the client. If the request
// did not have an EDNS record, nil is returned.
func (s *State) SizeAndDo() *dns.OPT {
	o := s.Opt()
	if o == nil {
		return nil
	}

	size := s.Size()
	Do := s.Do()
	o.SetVersion(0)
	// Options set in the request should still be there, don't fiddle with those.
	o.SetUDPSize(uint16(size))
	if Do {
		o.SetDo()
	}
	return o
}

// Result is the result of Scrub.
type Result int

const (
	// ScrubIgnored is returned when Scrub did nothing to the message.
	ScrubIgnored Result = iota
	// ScrubDone is returned when the reply has been scrubbed.
	ScrubDone
)

// Scrub scrubs the reply message so that it will fit the client's buffer. If even after dropping
// the additional section, it still does not fit the TC bit will be set on the message. Note,
// the TC bit will be set regardless of protocol, even TCP message will get the bit, the client
// should then retry with pigeons.
// TODO(referral).
func (s *State) Scrub(reply *dns.Msg) (*dns.Msg, Result) {
	size := s.Size()
	l := reply.Len()
	if size >= l {
		return reply, ScrubIgnored
	}
	// If not delegation, drop additional section.
	// TODO(miek): check for delegation

	reply.Extra = nil
	o := s.SizeAndDo() // re-add EDNS0 record
	if o != nil {
		reply.Extra = []dns.RR{o}
	}
	l = reply.Len()
	if size >= l {
		return reply, ScrubDone
	}
	// Still?!! does not fit.
	reply.Truncated = true
	return reply, ScrubDone
}

// Type returns the type of the question as a string.
func (s *State) Type() string { return dns.Type(s.Req.Question[0].Qtype).String() }

// QType returns the type of the question as an uint16.
func (s *State) QType() uint16 { return s.Req.Question[0].Qtype }

// Name returns the name of the question in the request. Note
// this name will always have a closing dot and will be lower cased.
func (s *State) Name() string { return strings.ToLower(dns.Name(s.Req.Question[0].Name).String()) }

// QName returns the name of the question in the request.
func (s *State) QName() string { return dns.Name(s.Req.Question[0].Name).String() }

// Class returns the class of the question in the request.
func (s *State) Class() string { return dns.Class(s.Req.Question[0].Qclass).String() }

// QClass returns the class of the question in the request.
func (s *State) QClass() uint16 { return s.Req.Question[0].Qclass }

// ErrorMessage returns an error message suitable for sending
// back to the client.
func (s *State) ErrorMessage(rcode int) *dns.Msg {
	m := new(dns.Msg)
	m.SetRcode(s.Req, rcode)
	return m
}
