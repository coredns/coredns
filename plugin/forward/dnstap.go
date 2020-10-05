package forward

import (
	"net"
	"strconv"
	"time"

	"github.com/coredns/coredns/plugin/dnstap/msg"
	"github.com/coredns/coredns/request"

	tap "github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
)

// toDnstap will send the forward and received message to the dnstap plugin.
func toDnstap(f *Forward, host string, state request.Request, reply *dns.Msg, start time.Time) {
	// Query
	tm := new(tap.Message)
	msg.SetQueryTime(tm, start)
	ip, p, _ := net.SplitHostPort(host)     // this is preparsed and can't err here
	port, _ := strconv.ParseUint(p, 10, 32) // same here

	opts := f.opts
	t := state.Proto()
	switch {
	case opts.forceTCP: // TCP flag has precedence over UDP flag
		t = "tcp"
	case opts.preferUDP:
		t = "udp"
	}

	if t == "tcp" {
		ta := &net.TCPAddr{IP: net.ParseIP(ip), Port: int(port)}
		msg.SetQueryAddress(tm, ta)
	} else {
		ta := &net.UDPAddr{IP: net.ParseIP(ip), Port: int(port)}
		msg.SetQueryAddress(tm, ta)
	}

	if f.tapPlugin.IncludeRawMessage {
		if buf, err := state.Req.Pack(); err != nil {
			tm.QueryMessage = buf
		}
	}
	msg.SetType(tm, tap.Message_FORWARDER_QUERY)
	f.tapPlugin.TapMessage(tm)

	// Response
	if reply != nil {
		if f.tapPlugin.IncludeRawMessage {
			if buf, err := reply.Pack(); err != nil {
				tm.ResponseMessage = buf
			}
		}
		tm.QueryMessage = nil // zero this, to not send it again
		msg.SetResponseTime(tm, time.Now())
		msg.SetType(tm, tap.Message_FORWARDER_RESPONSE)
		f.tapPlugin.TapMessage(tm)
	}
}
