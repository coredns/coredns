package strategy

import (
	"encoding/json"
	"net"
	"time"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type mid struct {
	req request.Request
	res *dns.Msg
}

func newMid() mid {
	return mid{
		req: request.Request{
			Req: &dns.Msg{},
		},
		res: &dns.Msg{},
	}
}

func (p mid) SetQuestion(q string, t uint16) {
	p.req.Req.SetQuestion(q, t)
}

func (p mid) SetSubnet(ip string) {
	opt := new(dns.OPT)
	opt.Hdr.Name = "."
	opt.Hdr.Rrtype = dns.TypeOPT
	e := new(dns.EDNS0_SUBNET)
	e.Address = net.ParseIP(ip)
	opt.Option = append(opt.Option, e)
	p.req.Req.Extra = append(p.req.Req.Extra, opt)
}

func (p mid) AddResponseAnswer(rr dns.RR) {
	p.res.Answer = append(p.res.Answer, rr)
}

func (p mid) AddResponseExtra(rr dns.RR) {
	p.res.Extra = append(p.res.Answer, rr)
}

func (p mid) AddRequestAnswer(rr dns.RR) {
	p.req.Req.Answer = append(p.req.Req.Answer, rr)
}

// adds raw value from slice of dns.RR
func (p mid) AddRequestOpt(rr ...dns.RR) {
	type state struct {
		IPs []string `json:"ip"`
	}
	json, _ := json.Marshal(state{IPs: getIPs(rr)})
	opt := new(dns.OPT)
	opt.Hdr.Name = "."
	opt.Hdr.Rrtype = dns.TypeOPT
	e := new(dns.EDNS0_LOCAL)
	e.Code = dns.EDNS0LOCALSTART
	e.Data = append([]byte("_rr_state="), json...)
	opt.Option = append(opt.Option, e)
	p.req.Req.Extra = append(p.req.Req.Extra, opt)
}

// adds raw value to OPT of the DNS query
func (p mid) AddRequestOptRaw(data string) {
	opt := new(dns.OPT)
	opt.Hdr.Name = "."
	opt.Hdr.Rrtype = dns.TypeOPT
	e := new(dns.EDNS0_LOCAL)
	e.Code = dns.EDNS0LOCALSTART
	e.Data = []byte(data)
	opt.Option = append(opt.Option, e)
	p.req.Req.Extra = append(p.req.Req.Extra, opt)
}

func getIPs(arr []dns.RR) (ips []string) {
	ips = []string{}
	for _, rr := range arr {
		switch rr.Header().Rrtype {
		case dns.TypeA:
			ips = append(ips, rr.(*dns.A).A.String())
		case dns.TypeAAAA:
			ips = append(ips, rr.(*dns.AAAA).AAAA.String())
		}
	}
	return
}

var filterAandAAAA = func(answers []dns.RR) (rr []dns.RR) {
	rrMap, ipSlice, _ := parseAnswerSection(answers)
	for _, ip := range ipSlice {
		rr = append(rr, rrMap[ip])
	}
	return
}

type stateFlatten struct {
	key       string
	question  string
	t         dnsType
	timestamp time.Time
	ips       []string
}

func buildState(tests []stateFlatten) mstate {
	m := make(mstate)
	for _, test := range tests {
		m.upsert(key(test.key), question(test.question), test.t, state{test.timestamp, test.ips})
	}
	return m
}
