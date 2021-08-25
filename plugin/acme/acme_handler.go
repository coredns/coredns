package acme

import (
	"context"
	"net"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

type AcmeHandler struct {
	Next     plugin.Handler
	provider *Provider
	*AcmeConfig
}

type AcmeConfig struct {
	Zone                    string
	Ipv4Addr                net.IP
	Ipv6Addr                net.IP
	AuthoritativeNameserver string
}

const (
	dnsChallengeString   = "_acme-challenge."
	certificateAuthority = "letsencrypt.org"
)

func (h AcmeHandler) Name() string { return pluginName }

func (h AcmeHandler) getQualifiedZone(zone string) string {
	if !strings.HasSuffix(zone, ".") {
		return zone + "."
	}
	return zone
}
func (h AcmeHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	a := new(dns.Msg)
	a.SetReply(state.Req)
	a.Answer = []dns.RR{}
	class := state.QClass()
	for _, question := range r.Question {
		zone := strings.ToLower(question.Name)
		if checkDNSChallenge(zone) {
			switch question.Qtype {
			case dns.TypeSOA:
				h.handleSOA(ctx, zone, class, a)
			case dns.TypeTXT:
				err := h.solveDNSChallenge(ctx, zone, class, a)
				if err != nil {
					log.Errorf("acmeHandler.solveDNSChallenge for zone %s err: %+v", zone, err)
					return 0, err
				}
			case dns.TypeNS:
				rr := new(dns.NS)
				rr.Ns = h.AuthoritativeNameserver
				rr.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeNS, Class: class}
				a.Answer = append(a.Answer, rr)
			case dns.TypeA:
				h.handleA(ctx, zone, class, a)
			case dns.TypeAAAA:
				h.handleAAAA(ctx, zone, class, a)
			}
		}
		if zone == h.getQualifiedZone(h.Zone) {
			switch question.Qtype {
			case dns.TypeSOA:
				h.handleSOA(ctx, zone, class, a)
			case dns.TypeCAA:
				rr := new(dns.CAA)
				rr.Tag = "issue"
				rr.Value = certificateAuthority
				rr.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeCAA, Class: class}
				a.Answer = append(a.Answer, rr)
			case dns.TypeA:
				h.handleA(ctx, zone, class, a)
			case dns.TypeAAAA:
				h.handleAAAA(ctx, zone, class, a)
			}
		}
	}
	if len(a.Answer) != 0 {
		err := w.WriteMsg(a)
		if err != nil {
			log.Error("acmeHandler.ServeDNS w.WriteMsg error: ", err)
			return 0, err
		}
	}

	return h.Next.ServeDNS(ctx, w, r)
}

func checkDNSChallenge(zone string) bool {
	return strings.HasPrefix(zone, dnsChallengeString)
}

func (h *AcmeHandler) solveDNSChallenge(ctx context.Context, zone string, class uint16, a *dns.Msg) error {
	a.Authoritative = true
	records, err := h.provider.GetRecords(ctx, zone)
	if err != nil {
		return err
	}
	rrs := []dns.RR{}
	log.Info(records)
	for _, record := range records {
		rr := new(dns.TXT)
		rr.Txt = []string{record.Value}
		rr.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeTXT, Class: class, Ttl: uint32(record.TTL)}
		rrs = append(rrs, rr)
	}
	a.Answer = append(a.Answer, rrs...)
	return nil
}

func (h *AcmeHandler) handleSOA(ctx context.Context, name string, class uint16, a *dns.Msg) {
	rr := new(dns.SOA)
	rr.Ns = h.AuthoritativeNameserver
	rr.Mbox = h.getQualifiedZone(certificateAuthority)
	rr.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeSOA, Class: class}
	rr.Serial = uint32(1)
	rr.Expire = uint32(60)
	rr.Minttl = uint32(60)
	a.Answer = append(a.Answer, rr)
}

func (h *AcmeHandler) handleA(ctx context.Context, name string, class uint16, a *dns.Msg) {
	rr := new(dns.A)
	rr.A = h.Ipv4Addr
	rr.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: class}
	a.Answer = append(a.Answer, rr)
}

func (h *AcmeHandler) handleAAAA(ctx context.Context, name string, class uint16, a *dns.Msg) {
	rr := new(dns.AAAA)
	rr.AAAA = h.Ipv6Addr
	rr.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA, Class: class}
	a.Answer = append(a.Answer, rr)
}
