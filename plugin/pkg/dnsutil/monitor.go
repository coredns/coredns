package dnsutil

import (
	"github.com/miekg/dns"
)

var monitorType = map[uint16]struct{}{
	dns.TypeAAAA:   {},
	dns.TypeA:      {},
	dns.TypeCNAME:  {},
	dns.TypeDNSKEY: {},
	dns.TypeDS:     {},
	dns.TypeMX:     {},
	dns.TypeNSEC3:  {},
	dns.TypeNSEC:   {},
	dns.TypeNS:     {},
	dns.TypePTR:    {},
	dns.TypeRRSIG:  {},
	dns.TypeSOA:    {},
	dns.TypeSRV:    {},
	dns.TypeTXT:    {},
	// Meta Qtypes
	dns.TypeIXFR: {},
	dns.TypeAXFR: {},
	dns.TypeANY:  {},
}

const other = "other"

// QTypeMonitorLabel returns dns type label based on a list of monitored types.
// Will return "other" for unmonitored ones.
func QTypeMonitorLabel(qtype uint16) string {
	if _, known := monitorType[qtype]; known {
		return dns.Type(qtype).String()
	}
	return other
}
