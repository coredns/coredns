package proxy

import (
	"net"

	"github.com/miekg/coredns/middleware/pkg/singleflight"
	"github.com/miekg/coredns/request"

	"github.com/miekg/dns"
)

type dnsUpstream struct {
	group *singleflight.Group
}

// ServeDNS does not satisfy middleware.Handler, instead it interacts with the upstream
// and returns the respons or an error.
func (d *dnsUpstream) Exchange(w dns.ResponseWriter, r *dns.Msg, u *UpstreamHost) (*dns.Msg, error) {
	co, err := net.DialTimeout(request.Proto(w), u.Name, defaultTimeout) // TODO(miek): timeout
	if err != nil {
		return nil, err
	}

	t := "nop"
	if t1, ok := dns.TypeToString[r.Question[0].Qtype]; ok {
		t = t1
	}
	cl := "nop"
	if cl1, ok := dns.ClassToString[r.Question[0].Qclass]; ok {
		cl = cl1
	}

	// Name needs to be normalized! Bug in go dns.
	rep, err := d.group.Do(r.Question[0].Name+t+cl, func() (interface{}, error) {
		ret, e := exchange(r, co)
		return ret, e
	})

	co.Close()

	if rep == nil {
		return nil, err

	}

	reply := rep.(*dns.Msg)
	// We need to unconditionally copy the message, becuase we don't know if it has been
	// shared within the group.Do from above....TODO(miek): should we fix this or not?
	copy := reply.Copy()

	if copy.Truncated {
		// Suppress proxy error for truncated responses.
		err = nil
	}

	copy.Compress = true
	copy.Id = r.Id

	return copy, err
}

func exchange(m *dns.Msg, co net.Conn) (*dns.Msg, error) {
	opt := m.IsEdns0()

	udpsize := uint16(dns.MinMsgSize)
	// If EDNS0 is used use that for size.
	if opt != nil && opt.UDPSize() >= dns.MinMsgSize {
		udpsize = opt.UDPSize()
	}

	dnsco := &dns.Conn{Conn: co, UDPSize: udpsize}

	dnsco.WriteMsg(m)
	r, err := dnsco.ReadMsg()
	dnsco.Close()

	return r, err
}
