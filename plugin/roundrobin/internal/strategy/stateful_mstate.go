package strategy

import (
	"fmt"
	"time"

	"github.com/miekg/dns"
)

// clientSubnet
type key string

// one client could hit many domains
type question string

type dnsType uint16

var dnsTypes = struct {
	A    dnsType
	AAAA dnsType
}{
	A:    dnsType(dns.TypeA),
	AAAA: dnsType(dns.TypeAAAA),
}

type state struct {
	timestamp time.Time
	ip        []string
}

type mstate map[key]map[question]map[dnsType]*state

// exists returns false if
func (m mstate) exists(k key, q question, t dnsType) (exists bool) {
	if _, ok := m[k]; ok {
		if _, ob := m[k][q]; ob {
			_, exists = m[k][q][t]
		}
	}
	return
}

// upsert add or insert new item to mstate
func (m mstate) upsert(k key, q question, t dnsType, s state) {
	if _, ok := m[k]; !ok {
		m[k] = make(map[question]map[dnsType]*state)
	}
	if _, ok := m[k][q]; !ok {
		m[k][q] = make(map[dnsType]*state)
	}
	m[k][q][t] = &s
}

func (m mstate) String() (out string) {
	for k, v := range m {
		for q, a := range v {
			for t, s := range a {
				out += fmt.Sprintf("[%v][%v][%s]{ips: %v} \n", k, q, t, s.ip)
			}
		}
	}
	return
}

func (t dnsType) String() string {
	if t == dnsTypes.AAAA {
		return "AAAA"
	}
	return "A"
}

// converts miekg.dnsType (uint16) to dnsType
func toDNSType(t uint16) dnsType {
	if t == dns.TypeAAAA {
		return dnsTypes.AAAA
	}
	return dnsTypes.A
}
