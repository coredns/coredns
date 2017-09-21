package healthcheck

import "github.com/miekg/dns"

// newPingMsg returns a dns.Msg that is used for the health check ping.
// The message's RD flag is not set. The name, type and class are
// "name", NS, IN repectively. Name must be fully qualified.
func newPingMsg(name string) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeNS)
	m.RecursionDesired = false
	return m
}
