// Package hanzodns implements a CoreDNS plugin that serves DNS records from
// an in-memory store managed via a REST API. Records can be created, updated,
// and deleted at runtime through the HTTP API, and are immediately available
// for DNS resolution.
package hanzodns

import (
	"context"
	"net"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// HanzoDNS is the CoreDNS plugin handler.
type HanzoDNS struct {
	Next  plugin.Handler
	Fall  fall.F
	Store *Store
	Addr  string // HTTP API listen address (default ":8443")
}

// Name implements plugin.Handler.
func (h *HanzoDNS) Name() string { return "hanzodns" }

// ServeDNS implements plugin.Handler. It looks up records in the store and
// writes an authoritative answer. If no records match it falls through to the
// next plugin.
func (h *HanzoDNS) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	qname := state.Name()
	qtype := state.QType()

	qtypeStr := qtypeToString(qtype)
	if qtypeStr == "" {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	// Check if the query is for a zone we manage.
	zones := h.Store.ZoneNames()
	zone := plugin.Zones(zones).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	records := h.Store.Lookup(qname, qtypeStr)
	if len(records) == 0 {
		if h.Fall.Through(qname) {
			return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
		}
		// Return NXDOMAIN-like via SERVFAIL (no SOA available in simple store).
		return dns.RcodeServerFailure, nil
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, rec := range records {
		rr := toRR(rec, qname)
		if rr != nil {
			m.Answer = append(m.Answer, rr)
		}
	}

	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// qtypeToString converts a dns.Type uint16 to the RecordType string.
func qtypeToString(qtype uint16) string {
	switch qtype {
	case dns.TypeA:
		return "A"
	case dns.TypeAAAA:
		return "AAAA"
	case dns.TypeCNAME:
		return "CNAME"
	case dns.TypeMX:
		return "MX"
	case dns.TypeTXT:
		return "TXT"
	case dns.TypeSRV:
		return "SRV"
	case dns.TypeNS:
		return "NS"
	case dns.TypeCAA:
		return "CAA"
	}
	return ""
}

// toRR converts a store Record to a dns.RR.
func toRR(rec Record, qname string) dns.RR {
	hdr := dns.RR_Header{
		Name:   qname,
		Rrtype: dns.StringToType[string(rec.Type)],
		Class:  dns.ClassINET,
		Ttl:    rec.TTL,
	}

	switch rec.Type {
	case TypeA:
		ip := net.ParseIP(rec.Content)
		if ip == nil {
			return nil
		}
		return &dns.A{Hdr: hdr, A: ip.To4()}

	case TypeAAAA:
		ip := net.ParseIP(rec.Content)
		if ip == nil {
			return nil
		}
		return &dns.AAAA{Hdr: hdr, AAAA: ip.To16()}

	case TypeCNAME:
		target := rec.Content
		if !strings.HasSuffix(target, ".") {
			target += "."
		}
		return &dns.CNAME{Hdr: hdr, Target: target}

	case TypeMX:
		host := rec.Content
		if !strings.HasSuffix(host, ".") {
			host += "."
		}
		return &dns.MX{Hdr: hdr, Preference: rec.Priority, Mx: host}

	case TypeTXT:
		return &dns.TXT{Hdr: hdr, Txt: []string{rec.Content}}

	case TypeSRV:
		// Content format: "weight port target"
		parts := strings.Fields(rec.Content)
		if len(parts) != 3 {
			return nil
		}
		weight, err := strconv.ParseUint(parts[0], 10, 16)
		if err != nil {
			return nil
		}
		port, err := strconv.ParseUint(parts[1], 10, 16)
		if err != nil {
			return nil
		}
		target := parts[2]
		if !strings.HasSuffix(target, ".") {
			target += "."
		}
		return &dns.SRV{
			Hdr:      hdr,
			Priority: rec.Priority,
			Weight:   uint16(weight),
			Port:     uint16(port),
			Target:   target,
		}

	case TypeNS:
		ns := rec.Content
		if !strings.HasSuffix(ns, ".") {
			ns += "."
		}
		return &dns.NS{Hdr: hdr, Ns: ns}

	case TypeCAA:
		// Content format: "flags tag value" e.g. "0 issue letsencrypt.org"
		parts := strings.SplitN(rec.Content, " ", 3)
		if len(parts) != 3 {
			return nil
		}
		flag, err := strconv.ParseUint(parts[0], 10, 8)
		if err != nil {
			return nil
		}
		return &dns.CAA{Hdr: hdr, Flag: uint8(flag), Tag: parts[1], Value: parts[2]}
	}

	return nil
}
