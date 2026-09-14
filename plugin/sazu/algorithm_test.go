package sazu

import (
	"testing"

	"github.com/miekg/dns"
)

func TestAlgorithmMeetsFloorAcceptsRecommendedAlgorithms(t *testing.T) {
	for _, alg := range []uint8{dns.RSASHA256, dns.ECDSAP256SHA256, dns.ECDSAP384SHA384, dns.ED25519, dns.ED448} {
		if !algorithmMeetsFloor(alg) {
			t.Errorf("algorithm %d: expected to meet the floor", alg)
		}
	}
}

func TestAlgorithmMeetsFloorRejectsWeakOrUnknownAlgorithms(t *testing.T) {
	for _, alg := range []uint8{
		dns.RSAMD5, dns.DSA, dns.RSASHA1, dns.DSANSEC3SHA1, dns.RSASHA1NSEC3SHA1,
		dns.RSASHA512, dns.ECCGOST, 0, 9, 11, 200,
	} {
		if algorithmMeetsFloor(alg) {
			t.Errorf("algorithm %d: expected to be below the floor", alg)
		}
	}
}
