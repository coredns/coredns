package sazu

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// RootHints are the 13 IANA root server addresses, "ip:53" ready to pass to
// a dns.Client. Pure data, hardcoded like the corresponding Rust/rDNS port
// -- no runtime parsing that could panic.
var RootHints = []string{
	"198.41.0.4:53",
	"199.9.14.201:53",
	"192.33.4.12:53",
	"199.7.91.13:53",
	"192.203.230.10:53",
	"192.5.5.241:53",
	"192.112.36.4:53",
	"198.97.190.53:53",
	"192.36.148.17:53",
	"192.58.128.30:53",
	"193.0.14.129:53",
	"199.7.83.42:53",
	"202.12.27.33:53",
}

// ChainError is returned by Validator methods. The Op field names which
// step of the chain walk failed, for callers that want to distinguish
// e.g. "no DS published yet" from "signature didn't verify" without
// string-matching Error().
type ChainError struct {
	Op  string
	Msg string
}

func (e *ChainError) Error() string { return fmt.Sprintf("sazu: chain-of-trust: %s: %s", e.Op, e.Msg) }

func chainErr(op, format string, args ...any) error {
	return &ChainError{Op: op, Msg: fmt.Sprintf(format, args...)}
}

// errNoDSRecords is returned internally by fetchAndVerifyDS when the
// parent answered authoritatively but currently publishes no DS at all
// for the queried name (an authoritative NOERROR/NODATA, not a failure).
// VerifyChainOfTrust checks for this specifically only on its *final*
// fetchAndVerifyDS call (the target zone's own parent), re-tagging it as
// ChainError{Op: "no-ds-published"} -- the single most common, expected
// state for a zone that has never been onboarded before, worth
// distinguishing from every other way the chain can fail (a broken
// ancestor, a network error, a key that doesn't match a DS that *does*
// exist) so a client can tell someone exactly what to do next instead of
// a bare "rejected."
var errNoDSRecords = errors.New("no DS records found")

// ChainValidator is the interface Sazu depends on for the §10.2
// chain-of-trust cross-check -- satisfied by *Validator, and small enough
// that tests can supply a fake implementation to exercise handler.go's
// response-shaping logic (in particular the ERR_NO_DS_PUBLISHED path)
// without making real network queries.
type ChainValidator interface {
	VerifyChainOfTrust(zone string, candidateKey *dns.DNSKEY) error
}

// Validator performs DNSSEC chain-of-trust validation from the hardcoded
// root trust anchor down to a zone's immediate parent -- the cryptographic
// check SAZU's design doc §10.2 calls the "chain-of-trust cross-check,"
// and that a first-contact SAZU push depends on for its security: without
// it, a DS lookup is just a plaintext UDP answer anyone off-path could
// forge, and "anchored to the root" would be a claim with nothing behind
// it.
//
// Scope, deliberately (see PREREQUISITES.md and the design doc's own
// scoping notes): this validates exactly what SAZU needs and nothing more.
//
//   - No NSEC/NSEC3 negative-answer proofs. SAZU only ever asks "does a DS
//     matching this key exist," never "prove no DS exists."
//   - No opportunistic multi-algorithm-set handling beyond trying every
//     RRSIG present until one verifies. Any break in the chain is a hard
//     failure, not a downgrade.
//   - The target zone's own DNSKEY RRset is never fetched or trusted here.
//     At first contact that zone may not be live/signed by anyone yet --
//     only the parent's DS RRset for it is validated.
//   - Uses its own small DO-bit-aware queries (via dns.Client) rather than
//     any of CoreDNS's existing resolving paths, so this has zero
//     interaction with, or risk to, ordinary query handling.
type Validator struct {
	Client *dns.Client
}

// NewValidator returns a Validator with a default 5s query timeout.
func NewValidator() *Validator {
	return &Validator{Client: &dns.Client{Timeout: 5 * time.Second}}
}

// VerifyChainOfTrust checks that candidateKey is anchored to the root for
// zone: the parent of zone publishes a DS record matching it, and every
// step of the delegation chain from the root down to that parent is
// cryptographically signed and matches the previous step's trust.
// Fail-closed: any missing record, invalid/expired signature, or network
// error is a validation failure, never a silent pass.
func (v *Validator) VerifyChainOfTrust(zone string, candidateKey *dns.DNSKEY) error {
	zone = dns.Fqdn(zone)
	if zone == "." {
		return chainErr("verify", "cannot validate the root zone itself")
	}
	labels := dns.SplitDomainName(zone)
	if len(labels) == 0 {
		return chainErr("verify", "zone %q has no labels", zone)
	}

	// Root -> TLD -> ... -> immediate parent, in descent order. For
	// "example.org." (labels ["example","org"]) this is just ["org."];
	// for a bare TLD target this is empty and the loop below is skipped
	// entirely, since the TLD's own parent is the root itself.
	var ancestors []string
	for i := len(labels) - 1; i >= 1; i-- {
		ancestors = append(ancestors, dns.Fqdn(strings.Join(labels[i:], ".")))
	}

	// Bootstrap: the root's own DNSKEY RRset, trusted directly via the
	// hardcoded anchor rather than a DS record (the root has no parent).
	servers := RootHints
	trustedKeys, err := v.fetchAndVerifyDNSKEYAtRoot(servers)
	if err != nil {
		return err
	}

	for _, ancestor := range ancestors {
		ds, err := v.fetchAndVerifyDS(ancestor, servers, trustedKeys)
		if err != nil {
			return err
		}
		servers, err = v.fetchDelegationServers(ancestor, servers)
		if err != nil {
			return err
		}
		trustedKeys, err = v.fetchAndVerifyDNSKEY(ancestor, servers, ds)
		if err != nil {
			return err
		}
	}

	// Final step: the immediate parent's DS answer for the actual target
	// zone. This is what the candidate key must match -- no fetch of the
	// target zone's own DNSKEY set.
	finalDS, err := v.fetchAndVerifyDS(zone, servers, trustedKeys)
	if err != nil {
		if errors.Is(err, errNoDSRecords) {
			return &ChainError{Op: "no-ds-published", Msg: fmt.Sprintf("no DS record published yet for %s", zone)}
		}
		return err
	}
	for _, ds := range finalDS {
		if dsMatchesKey(ds, candidateKey) {
			return nil
		}
	}
	// A DS *is* published for zone -- just not one matching this key.
	// Tagged separately from a generic "verify" failure because it needs
	// its own diagnostic (handler.go's ERR_UNKNOWN_SIGNER): the DS found
	// here isn't necessarily an attacker's. It's just as likely to be
	// pre-existing DNSSEC this zone's current host already publishes
	// (its own key, unrelated to SAZU) -- something a client needs to
	// know about before doing anything that might disturb it.
	return &ChainError{Op: "key-mismatch", Msg: fmt.Sprintf("a DS record is published for %s, but none of them match the candidate key", zone)}
}

// dsMatchesKey reports whether ds is the DS record for key, recomputing
// key's digest with ds's own digest algorithm rather than assuming SHA-256.
func dsMatchesKey(ds *dns.DS, key *dns.DNSKEY) bool {
	candidate := key.ToDS(ds.DigestType)
	if candidate == nil {
		return false
	}
	return candidate.KeyTag == ds.KeyTag &&
		candidate.Algorithm == ds.Algorithm &&
		strings.EqualFold(candidate.Digest, ds.Digest)
}

// fetchAndVerifyDNSKEYAtRoot fetches the root's DNSKEY RRset, verifies it
// against the hardcoded trust anchor and its own RRSIG, and returns the
// trusted set.
func (v *Validator) fetchAndVerifyDNSKEYAtRoot(servers []string) ([]*dns.DNSKEY, error) {
	resp, err := v.queryDO(".", dns.TypeDNSKEY, servers)
	if err != nil {
		return nil, err
	}
	if !resp.Authoritative || resp.Rcode != dns.RcodeSuccess {
		return nil, chainErr("root-dnskey", "root did not answer authoritatively for its own DNSKEY")
	}
	keys, sigs := splitKeysAndSigs(resp.Answer)
	if len(keys) == 0 {
		return nil, chainErr("root-dnskey", "no DNSKEY records returned for the root")
	}

	anchored := false
	for _, k := range keys {
		for _, a := range RootTrustAnchors() {
			if a.Matches(k) {
				anchored = true
			}
		}
	}
	if !anchored {
		return nil, chainErr("root-dnskey", "no root DNSKEY matches the pinned trust anchor")
	}

	if err := verifyAnyRRSIG(".", toRR(keys), dns.TypeDNSKEY, sigs, keys); err != nil {
		return nil, chainErr("root-dnskey", "%v", err)
	}
	return keys, nil
}

// fetchAndVerifyDNSKEY fetches zone's own DNSKEY RRset and verifies it
// against a DS RRset already trusted from its parent: a matching key must
// be present, and the whole RRset must carry a verifying RRSIG from a key
// in the same set.
func (v *Validator) fetchAndVerifyDNSKEY(zone string, servers []string, trustedDS []*dns.DS) ([]*dns.DNSKEY, error) {
	resp, err := v.queryDO(zone, dns.TypeDNSKEY, servers)
	if err != nil {
		return nil, err
	}
	if !resp.Authoritative || resp.Rcode != dns.RcodeSuccess {
		return nil, chainErr("dnskey", "%s did not answer authoritatively for its own DNSKEY", zone)
	}
	keys, sigs := splitKeysAndSigs(resp.Answer)
	if len(keys) == 0 {
		return nil, chainErr("dnskey", "no DNSKEY records found for %s", zone)
	}

	matched := false
	for _, k := range keys {
		for _, ds := range trustedDS {
			if dsMatchesKey(ds, k) {
				matched = true
			}
		}
	}
	if !matched {
		return nil, chainErr("dnskey", "no DNSKEY in %s's key set matches the trusted DS", zone)
	}

	if err := verifyAnyRRSIG(zone, toRR(keys), dns.TypeDNSKEY, sigs, keys); err != nil {
		return nil, chainErr("dnskey", "%s: %v", zone, err)
	}
	return keys, nil
}

// fetchAndVerifyDS fetches the DS RRset child published *at* servers (the
// parent zone's own authoritative servers -- DS records live in the
// parent, signed by the parent's key set), using the already-trusted
// trustedKeys.
func (v *Validator) fetchAndVerifyDS(child string, servers []string, trustedKeys []*dns.DNSKEY) ([]*dns.DS, error) {
	resp, err := v.queryDO(child, dns.TypeDS, servers)
	if err != nil {
		return nil, err
	}
	if !resp.Authoritative || resp.Rcode != dns.RcodeSuccess {
		return nil, chainErr("ds", "%s did not answer authoritatively for its own DS", child)
	}
	dsRecords, sigs := splitDSAndSigs(resp.Answer)
	if len(dsRecords) == 0 {
		return nil, fmt.Errorf("%s: %w", child, errNoDSRecords)
	}

	if err := verifyAnyRRSIG(child, toRR(dsRecords), dns.TypeDS, sigs, trustedKeys); err != nil {
		return nil, chainErr("ds", "%s: %v", child, err)
	}
	return dsRecords, nil
}

// fetchDelegationServers follows a referral for zone at parentServers to
// find zone's own authoritative server addresses (NS + in-bailiwick glue).
// No DO bit needed here -- NS is never a DNSSEC-covered type, so there is
// no signature to check regardless.
func (v *Validator) fetchDelegationServers(zone string, parentServers []string) ([]string, error) {
	m := new(dns.Msg)
	m.SetQuestion(zone, dns.TypeNS)
	m.RecursionDesired = false

	var resp *dns.Msg
	var lastErr error
	for _, server := range parentServers {
		r, _, err := v.Client.Exchange(m, server)
		if err != nil {
			lastErr = err
			continue
		}
		resp = r
		break
	}
	if resp == nil {
		return nil, chainErr("delegation", "no server for %s answered an NS query: %v", zone, lastErr)
	}

	nsNames := map[string]bool{}
	for _, rr := range concatRR(resp.Answer, resp.Ns) {
		if ns, ok := rr.(*dns.NS); ok && strings.EqualFold(ns.Hdr.Name, zone) {
			nsNames[strings.ToLower(ns.Ns)] = true
		}
	}
	if len(nsNames) == 0 {
		return nil, chainErr("delegation", "no NS records found to reach %s's own servers", zone)
	}

	var addrs []string
	for _, rr := range concatRR(resp.Answer, resp.Extra) {
		switch rec := rr.(type) {
		case *dns.A:
			if nsNames[strings.ToLower(rec.Hdr.Name)] {
				addrs = append(addrs, net.JoinHostPort(rec.A.String(), "53"))
			}
		case *dns.AAAA:
			if nsNames[strings.ToLower(rec.Hdr.Name)] {
				addrs = append(addrs, net.JoinHostPort(rec.AAAA.String(), "53"))
			}
		}
	}
	if len(addrs) == 0 {
		return nil, chainErr("delegation", "no glue records found to reach %s's own servers", zone)
	}
	return addrs, nil
}

// queryDO sends one EDNS0-DO query for qtype at name to each of servers in
// turn, returning the first well-formed reply.
func (v *Validator) queryDO(name string, qtype uint16, servers []string) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.SetEdns0(4096, true)
	m.RecursionDesired = false

	var lastErr error
	for _, server := range servers {
		resp, _, err := v.Client.Exchange(m, server)
		if err != nil {
			lastErr = err
			continue
		}
		return resp, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no servers given")
	}
	return nil, chainErr("query", "no authoritative server for %s/%d answered: %v", name, qtype, lastErr)
}

// verifyAnyRRSIG tries every RRSIG in sigs against every key in keyset
// whose tag/algorithm match, until one verifies (RFC 4034 canonicalization
// and the actual crypto check are both handled by RRSIG.Verify itself).
// Returns nil on the first success, an error if every combination was
// tried and none verified or matched the validity window.
func verifyAnyRRSIG(owner string, rrset []dns.RR, covered uint16, sigs []*dns.RRSIG, keyset []*dns.DNSKEY) error {
	if len(rrset) == 0 {
		return fmt.Errorf("empty rrset for %s", owner)
	}
	now := time.Now()
	for _, sig := range sigs {
		if sig.TypeCovered != covered || !sig.ValidityPeriod(now) {
			continue
		}
		for _, key := range keyset {
			if key.KeyTag() != sig.KeyTag || key.Algorithm != sig.Algorithm {
				continue
			}
			if err := sig.Verify(key, rrset); err == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("no RRSIG for %s verifies against the trusted key set", owner)
}

func splitKeysAndSigs(rrs []dns.RR) ([]*dns.DNSKEY, []*dns.RRSIG) {
	var keys []*dns.DNSKEY
	var sigs []*dns.RRSIG
	for _, rr := range rrs {
		switch v := rr.(type) {
		case *dns.DNSKEY:
			keys = append(keys, v)
		case *dns.RRSIG:
			if v.TypeCovered == dns.TypeDNSKEY {
				sigs = append(sigs, v)
			}
		}
	}
	return keys, sigs
}

func splitDSAndSigs(rrs []dns.RR) ([]*dns.DS, []*dns.RRSIG) {
	var ds []*dns.DS
	var sigs []*dns.RRSIG
	for _, rr := range rrs {
		switch v := rr.(type) {
		case *dns.DS:
			ds = append(ds, v)
		case *dns.RRSIG:
			if v.TypeCovered == dns.TypeDS {
				sigs = append(sigs, v)
			}
		}
	}
	return ds, sigs
}

// toRR upcasts a slice of a concrete RR type to []dns.RR, which is what
// RRSIG.Verify and IsRRset expect.
func toRR[T dns.RR](items []T) []dns.RR {
	out := make([]dns.RR, len(items))
	for i, it := range items {
		out[i] = it
	}
	return out
}

func concatRR(a, b []dns.RR) []dns.RR {
	out := make([]dns.RR, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}
