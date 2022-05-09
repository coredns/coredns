package strategy

import (
	"fmt"
	"time"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

const (
	missingSubnet = "missing-subnet"
	emptySubnet   = "empty-subnet"
)

type stateful struct {
	state mstate
}

func newStateful() *stateful {
	this := new(stateful)
	this.state = make(mstate)
	gc := newGarbageCollector(this.state, garbageCollectionDefaultTTLSeconds)
	go func() {
		for range time.Tick(time.Second * garbageCollectionPeriodSeconds) {
			gc.collect()
		}
	}()
	return this
}

func (s *stateful) update(req *request.Request, res *dns.Msg) (rr []dns.RR, err error) {
	if req == nil {
		err = fmt.Errorf("nil request")
		return
	}
	if res == nil {
		err = fmt.Errorf("nil response")
		return
	}
	if len(req.Req.Question) == 0 {
		err = fmt.Errorf("empty request question")
		return
	}
	return s.updateState(req, res)
}

func (s *stateful) updateState(req *request.Request, res *dns.Msg) (answer []dns.RR, err error) {
	q := question(req.Req.Question[0].Name)
	k := s.key(req)
	t := toDNSType(req.Req.Question[0].Qtype)
	responseIPsTable, responseIPs, responseNoIPs := parseAnswerSection(res.Answer)
	s.refresh(k, q, t, responseIPsTable, responseIPs)
	for _, ip := range s.state[k][q][t].ip {
		answer = append(answer, responseIPsTable[ip])
	}
	return append(answer, responseNoIPs...), nil
}

func (s *stateful) key(req *request.Request) key {
	subnet := s.readSubnet(req.Req)
	return key(subnet)
}

// readSubnet reads the option EDNS0_SUBNET which is usually filled by resolvers.
func (s *stateful) readSubnet(req *dns.Msg) string {
	for _, e := range req.Extra {
		opt := e.(*dns.OPT)
		if opt == nil {
			continue
		}
		for _, o := range opt.Option {
			x := o.(*dns.EDNS0_SUBNET)
			if x == nil {
				continue
			}
			if o.(*dns.EDNS0_SUBNET).Address.String() == "<nil>" {
				return emptySubnet
			}
			return o.(*dns.EDNS0_SUBNET).Address.String()
		}
	}
	return missingSubnet
}

func (s *stateful) refresh(k key, q question, t dnsType, responseA map[string]dns.RR, responseIPs []string) {
	if !s.state.exists(k, q, t) {
		s.state.upsert(k, q, t, state{ip: []string{}, timestamp: time.Now()})
	}
	s.state[k][q][t].updateState(responseA, responseIPs)
	s.state[k][q][t].rotateIPs()
}

func (s *state) updateState(responseA map[string]dns.RR, responseIPs []string) {
	var newIPs []string
	currentA := ipsToSet(s.ip)

	// append only such IP which exist in response
	for _, ip := range s.ip {
		if _, found := responseA[ip]; found {
			newIPs = append(newIPs, ip)
		}
	}

	// to the end of the IP list append new records which doesn't exist in request but exist in response.
	for _, ip := range responseIPs {
		if !currentA[ip] {
			newIPs = append(newIPs, ip)
		}
	}
	s.ip = newIPs
	s.timestamp = time.Now()
}

func (s *state) rotateIPs() {
	s.ip = rotate(s.ip)
}
