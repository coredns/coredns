package record_test

import (
	"testing"

	"github.com/coredns/coredns/plugin/atlas/ent"
	"github.com/coredns/coredns/plugin/atlas/record"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestGetRRHeaderFromDnsRR_NilRec(t *testing.T) {
	hdr, err := record.GetRRHeaderFromDnsRR(nil)
	require.Nil(t, hdr)
	require.NotNil(t, err)
	require.Equal(t, "unexpected atlas resource record", err.Error())
}

func TestGetRRHeaderFromDnsRR_Rec(t *testing.T) {
	entRec := &ent.DnsRR{
		Name:   "bla.com.",
		Rrtype: dns.TypeTXT,
		Class:  dns.ClassINET,
		TTL:    360,
	}
	hdr, err := record.GetRRHeaderFromDnsRR(entRec)
	require.NotNil(t, hdr)
	require.Nil(t, err)
	require.Equal(t, "bla.com.", hdr.Name)
	require.Equal(t, dns.TypeTXT, hdr.Rrtype)

	// ClassINET is defined as:
	// const dns.ClassINET untyped int = 1
	//
	// and in header
	// type RR_Header struct {
	//		...
	// 		Class    uint16
	// 		...
	// }
	// so we have to cast to the correct type to pass the test :(
	require.Equal(t, uint16(dns.ClassINET), hdr.Class)
	require.Equal(t, uint32(360), hdr.Ttl)
}
