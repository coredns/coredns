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

// oDnstap will send the forward and received message to the dnstap plugin.
func toDnstap(f *Forward, host string, state request.Request, opts options, reply *dns.Msg, start time.Time) {
	// Query
	q := new(tap.Message)
	msg.SetQueryTime(q, start)
	ip, p, _ := net.SplitHostPort(host)     // this is preparsed and can't err here
	port, _ := strconv.ParseUint(p, 10, 32) // same here

	t := state.Proto()
	switch {
	case opts.forceTCP: // TCP flag has precedence over UDP flag
		t = "tcp"
	case opts.preferUDP:
		t = "udp"
	}

	if t == "tcp" {
		ta := &net.TCPAddr{IP: net.ParseIP(ip), Port: int(port)}
		msg.SetQueryAddress(q, ta)
	} else {
		ta := &net.UDPAddr{IP: net.ParseIP(ip), Port: int(port)}
		msg.SetQueryAddress(q, ta)
	}

	if f.tapPlugin.IncludeRawMessage {
		buf, _ := state.Req.Pack()
		q.QueryMessage = buf
	}
	msg.SetType(q, tap.Message_FORWARDER_QUERY)
	f.tapPlugin.TapMessage(q)

	// Response
	r := new(tap.Message)
	if reply != nil {
		if f.tapPlugin.IncludeRawMessage {
			buf, _ := reply.Pack()
			r.ResponseMessage = buf
		}
		msg.SetQueryTime(r, start)
		msg.SetResponseTime(r, time.Now())
		msg.SetType(r, tap.Message_FORWARDER_RESPONSE)
		f.tapPlugin.TapMessage(r)
	}
}
