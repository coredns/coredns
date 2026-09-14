package sazu

import "github.com/miekg/dns"

// algorithmFloor is SAZU's §10.7 minimum DNSSEC algorithm policy: the set
// of DNSKEY algorithms a candidate key is allowed to onboard with, at all.
// An allowlist, not a denylist -- an unrecognized or future algorithm
// number is refused by default rather than silently accepted, which is
// the safer failure direction for a security policy like this one.
//
// Values and their status come from RFC 8624 §3.1's "Algorithm
// Implementation Requirements and Usage Guidance for DNSSEC" table (the
// "Zone Signing" column): this package accepts anything rated MUST, MAY,
// or RECOMMENDED there, and refuses everything rated MUST NOT or NOT
// RECOMMENDED. Concretely: RSAMD5 (1), DSA/SHA1 (3), RSASHA1 (5),
// DSA-NSEC3-SHA1 (6), RSASHA1-NSEC3-SHA1 (7), RSASHA512 (10, "NOT
// RECOMMENDED" -- surprising at first glance for a SHA-512-based
// algorithm, but RFC 8624 downgrades it because RSA/SHA-512 signatures
// are needlessly large for no real security benefit over RSASHA256, and
// ECC-GOST (12) are all refused; RSASHA256 (8), ECDSAP256SHA256 (13),
// ECDSAP384SHA384 (14), ED25519 (15, this package's own default -- see
// key.go), and ED448 (16) are all accepted.
//
// This is the Go port's counterpart to the earlier Rust/rDNS port's
// meets_minimum_floor() check, which did not carry over when this
// package was first written -- see SAZU-PLAN.md.
var algorithmFloor = map[uint8]bool{
	dns.RSASHA256:       true,
	dns.ECDSAP256SHA256: true,
	dns.ECDSAP384SHA384: true,
	dns.ED25519:         true,
	dns.ED448:           true,
}

// algorithmMeetsFloor reports whether algorithm (a DNSKEY's Algorithm
// field) is one SAZU will accept a candidate key onboarding with.
func algorithmMeetsFloor(algorithm uint8) bool {
	return algorithmFloor[algorithm]
}
