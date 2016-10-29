package middleware

import (
	"github.com/miekg/coredns/middleware/etcd/msg"

	"github.com/miekg/dns"
)

// ServicesToTxt puts debug in TXT RRs.
func ServicesToTxt(debug []msg.Service) []dns.RR {
	if debug == nil {
		return nil
	}

	rr := make([]dns.RR, len(debug))
	for i, d := range debug {
		rr[i] = d.RR()
	}
	return rr
}

// ErrorToText puts in error's text into an TXT RR.
func ErrorToTxt(err error) dns.RR {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if len(msg) > 255 {
		msg = msg[:255]
	}
	t := new(dns.TXT)
	t.Hdr.Class = dns.ClassCHAOS
	t.Hdr.Ttl = 0
	t.Hdr.Rrtype = dns.TypeTXT
	t.Hdr.Name = "."

	t.Txt = []string{msg}
	return t
}
