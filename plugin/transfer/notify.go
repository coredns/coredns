package transfer

import (
	"fmt"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/rcode"

	"github.com/miekg/dns"
)

// Notify sends notifies for incoming messages from plugins
func (t Transfer) Notify(data <-chan []string, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case zones := <-data:
			for _, zone := range zones {
				t.sendNotifies(zone)
			}
		}
	}
}

// sendNotifies sends notifies to the configured remote servers for the transfer instance handling the zone.
// It will try up to three times before giving up on a specific remote. It will sequentially loop
// through "to" until they all have replied (or have 3 failed attempts).
func (t Transfer) sendNotifies(zone string) {
	var to []string

	// get remote servers for this zone
	for _, x := range t.xfrs {
		matchZone := plugin.Zones(x.Zones).Matches(zone)
		if matchZone == "" {
			continue
		}
		to = x.to
		break
	}
	if len(to) == 0 {
		return
	}

	m := new(dns.Msg)
	m.SetNotify(zone)
	c := new(dns.Client)

	for _, t := range to {
		if t == "*" {
			continue
		}
		if err := notifyAddr(c, m, t); err != nil {
			log.Error(err.Error())
		}
	}
	log.Infof("Sent notifies for zone %q to %v", zone, to)
}

func notifyAddr(c *dns.Client, m *dns.Msg, s string) error {
	var (
		err error
		ret *dns.Msg
	)
	code := dns.RcodeServerFailure
	for i := 0; i < 3; i++ {

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
