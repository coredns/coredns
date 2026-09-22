package nomad

import (
	"net"

	"github.com/hashicorp/nomad/api"
	"github.com/miekg/dns"
)

func addSRVRecord(m *dns.Msg, s *api.ServiceRegistration, header dns.RR_Header, originalQName string, addr net.IP, ttl uint32) error {
	srvRecord := &dns.SRV{
		Hdr:      header,
		Target:   originalQName,
		Port:     uint16(s.Port), // #nosec G115 -- port numbers are bounded (1-65535)
		Priority: 10,
		Weight:   10,
	}
	m.Answer = append(m.Answer, srvRecord)

	if addr.To4() == nil {
		addExtrasToAAAARecord(m, originalQName, ttl, addr)
	} else {
		addExtrasToARecord(m, originalQName, ttl, addr)
	}

	return nil
}

func addExtrasToARecord(m *dns.Msg, originalQName string, ttl uint32, addr net.IP) {
	header := dns.RR_Header{
		Name:   originalQName,
		Rrtype: dns.TypeA,
		Class:  dns.ClassINET,
		Ttl:    ttl,
	}
	m.Extra = append(m.Extra, &dns.A{Hdr: header, A: addr})
}

func addExtrasToAAAARecord(m *dns.Msg, originalQName string, ttl uint32, addr net.IP) {
	header := dns.RR_Header{
		Name:   originalQName,
		Rrtype: dns.TypeAAAA,
		Class:  dns.ClassINET,
		Ttl:    ttl,
	}
	m.Extra = append(m.Extra, &dns.AAAA{Hdr: header, AAAA: addr})
}

func addARecord(m *dns.Msg, header dns.RR_Header, addr net.IP) {
	m.Answer = append(m.Answer, &dns.A{Hdr: header, A: addr})
}

func addAAAARecord(m *dns.Msg, header dns.RR_Header, addr net.IP) {
	m.Answer = append(m.Answer, &dns.AAAA{Hdr: header, AAAA: addr})
}

func createSOARecord(originalQName string, ttl uint32, zone string) *dns.SOA {
	return &dns.SOA{
		Hdr:     dns.RR_Header{Name: dns.Fqdn(originalQName), Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: ttl},
		Ns:      dns.Fqdn("ns1." + originalQName),
		Mbox:    dns.Fqdn("hostmaster." + zone),
		Serial:  0,
		Refresh: 3600,
		Retry:   600,
		Expire:  86400,
		Minttl:  30,
	}
}
