package transfer

import (
	"fmt"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin/pkg/rcode"

	"github.com/miekg/dns"
)

// RFC8945 recommended value
const defaultFudge = 300

// Notify will send notifies to all configured to hosts IP addresses. The string zone must be lowercased.
func (t *Transfer) Notify(zone string) error {
	if t == nil { // t might be nil, mostly expected in tests, so intercept and to a noop in that case
		return nil
	}

	m := new(dns.Msg)
	m.SetNotify(zone)
	c := new(dns.Client)

	x := longestMatch(t.xfrs, zone)
	if x == nil {
		// return without error if there is no matching zone
		return nil
	}

	if tsigName := t.getTsigName(); tsigName != "" {
		m.SetTsig(tsigName, t.tsigAlgorithm[tsigName], defaultFudge, time.Now().Unix())
		c.TsigProvider = dnsserver.NewTsigProvider(t.tsigSecret, t.tsigAlgorithm)
	}

	var err1 error
	for _, t := range x.to {
		if t == "*" {
			continue
		}
		if err := sendNotify(c, m, t); err != nil {
			err1 = err
		}
	}
	log.Debugf("Sent notifies for zone %q to %v", zone, x.to)
	return err1 // this only captures the last error
}

func sendNotify(c *dns.Client, m *dns.Msg, s string) error {
	var err error
	var ret *dns.Msg

	code := dns.RcodeServerFailure
	for range 3 {
		ret, _, err = c.Exchange(m, s)
		if err != nil {
			continue
		}
		code = ret.Rcode
		if code == dns.RcodeSuccess {
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("notify for zone %q was not accepted by %q: %q", m.Question[0].Name, s, err)
	}
	return fmt.Errorf("notify for zone %q was not accepted by %q: rcode was %q", m.Question[0].Name, s, rcode.ToString(code))
}

func (t *Transfer) getTsigName() string {
	for name := range t.tsigSecret {
		return name
	}
	return ""
}
