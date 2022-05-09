package strategy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/miekg/dns"
)

type stateless struct {
	IPs []string `json:"ip"`
	// IPs converted into map
	requestIPs map[string]bool
	// response.Answers[] converted into map which are A or AAAA. IP is a key
	responseA map[string]dns.RR
	// contains all response IPs which are A or AAAA
	responseIPs []string
	// contains all response records which are not A nor AAAA
	responseNoA []dns.RR
	err         error
}

func newStateless(request *dns.Msg, response *dns.Msg) (s *stateless) {
	const identifierPrefix = "_rr_state="
	var empty = func() *stateless {
		return &stateless{
			requestIPs:  map[string]bool{},
			responseA:   map[string]dns.RR{},
			responseIPs: []string{},
			responseNoA: []dns.RR{},
		}
	}
	if request == nil || response == nil {
		s.err = fmt.Errorf("nil response or request")
		return
	}
	s = empty()
	// extracting records from client request
	for _, e := range request.Extra {
		opt := e.(*dns.OPT)
		if opt == nil {
			continue
		}
		for _, o := range opt.Option {
			e := o.(*dns.EDNS0_LOCAL)
			if e == nil {
				continue
			}
			data := string(o.(*dns.EDNS0_LOCAL).Data)
			if strings.HasPrefix(data, identifierPrefix) {
				str := strings.ReplaceAll(data, identifierPrefix, "")
				_ = json.Unmarshal([]byte(str), &s)
				if s == nil {
					s = empty()
				}
			}
		}
	}
	s.responseA, s.responseIPs, s.responseNoA = parseAnswerSection(response.Answer)
	s.requestIPs = ipsToSet(s.IPs)
	return s
}

// updateState compare stateless records with response message records
// and cuts removed records or append new records to stateless
// updateState keeps records in the same order as they ar defined in the IPs field.
func (s *stateless) updateState() *stateless {
	var newIPs []string

	// append only such IP which exist in response
	for _, ip := range s.IPs {
		if _, found := s.responseA[ip]; found {
			newIPs = append(newIPs, ip)
		}
	}

	// to the end of the IP list append new records which doesn't exist in OPT but exist in response.
	for _, ip := range s.responseIPs {
		if !s.requestIPs[ip] {
			newIPs = append(newIPs, ip)
		}
	}

	s.IPs = newIPs
	return s
}

// rotate performs a cyclic rotation of the IPs records
func (s *stateless) rotate() *stateless {
	s.IPs = rotate(s.IPs)
	return s
}

// getAnswers recreates Answer slice from original answers
func (s *stateless) getAnswers() ([]dns.RR, error) {
	var shuffled []dns.RR
	for _, ip := range s.IPs {
		shuffled = append(shuffled, s.responseA[ip])
	}
	return append(shuffled, s.responseNoA...), s.err
}
