// Command sazuctl is a minimal test client for the SAZU push protocol --
// the Go/CoreDNS counterpart to the earlier Rust/rDNS port's sazu-client.
// It builds a first-contact SAZU push (a DNSKEY add for the client's own
// key, plus one A record), signs it with SIG(0) (RFC 2931) using that same
// key -- the design's central decision (§9.1: one key does both jobs) --
// and either sends it to a target over UDP or just prints/self-verifies
// it.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/coredns/coredns/plugin/sazu"

	"github.com/miekg/dns"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "keygen":
		err = runKeygen(os.Args[2:])
	case "ds":
		err = runDS(os.Args[2:])
	case "push":
		err = runPush(os.Args[2:])
	case "push-zone":
		err = runPushZone(os.Args[2:])
	case "push-update":
		err = runPushUpdate(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "sazuctl: error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: sazuctl <keygen|ds|push|push-zone> [flags]")
	fmt.Fprintln(os.Stderr, "  sazuctl keygen -out <path> [-zone <owner>]")
	fmt.Fprintln(os.Stderr, "  sazuctl ds -zone <zone> -key <path>")
	fmt.Fprintln(os.Stderr, "  sazuctl push -zone <zone> -key <path> [-record name=ipv4] [-ttl 300] [-target host:port]")
	fmt.Fprintln(os.Stderr, "  sazuctl push-zone -zone <zone> -key <path> -zonefile <path> [-previous-serial N] [-target host:port]")
	fmt.Fprintln(os.Stderr, "  sazuctl push-zone -zone <zone> -key <path> [-ns <nsname>] [-add \"rr\"]... [-target host:port]  (no zone file needed)")
	fmt.Fprintln(os.Stderr, "  sazuctl push-update -zone <zone> -key <path> [-add \"rr\"]... [-del \"rr\"]... [-del-rrset \"name TYPE\"]... [-target host:port]")
}

// stringSliceFlag collects a repeatable -flag value1 -flag value2 ... into
// a slice, since the standard flag package has no built-in repeatable
// string flag type.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func runKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", "", "path to write the new key to")
	zone := fs.String("zone", "example.org", "owner name for the key (cosmetic until push)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("-out is required")
	}

	key, priv, err := sazu.GenerateEd25519Key(*zone, true)
	if err != nil {
		return err
	}
	if err := sazu.SavePrivateKey(*out, key, priv); err != nil {
		return err
	}
	printKeyInfo(*out, key)
	return nil
}

func runDS(args []string) error {
	fs := flag.NewFlagSet("ds", flag.ExitOnError)
	zone := fs.String("zone", "", "zone this key is for")
	keyPath := fs.String("key", "", "path to the Ed25519 key (created if missing)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *zone == "" || *keyPath == "" {
		return fmt.Errorf("-zone and -key are required")
	}

	key, _, generated, err := sazu.LoadOrGenerateKey(*keyPath, *zone, true)
	if err != nil {
		return err
	}
	if generated {
		fmt.Fprintf(os.Stderr, "No key found at %s -- generated a new one.\n", *keyPath)
	}

	ds := key.ToDS(dns.SHA256)
	fmt.Printf("DS record for %s -- give this to your registrar/parent zone:\n\n", *zone)
	fmt.Printf("  %s IN DS %d %d %d %s\n\n", key.Hdr.Name, ds.KeyTag, ds.Algorithm, ds.DigestType, ds.Digest)
	fmt.Printf("  key tag:     %d\n", ds.KeyTag)
	fmt.Printf("  algorithm:   %d (%s)\n", ds.Algorithm, algorithmLabel(key.Algorithm))
	fmt.Printf("  digest type: %d (SHA-256)\n", ds.DigestType)
	fmt.Printf("  digest:      %s\n\n", ds.Digest)
	fmt.Println("Some registrars (e.g. AWS Route 53) ask for the raw public key")
	fmt.Println("and its flags instead of, or in addition to, a DS record:")
	fmt.Println()
	fmt.Printf("  public key type: %d (%s)\n", key.Flags, keyTypeLabel(key.Flags))
	fmt.Printf("  public key:      %s\n", key.PublicKey)
	return nil
}

func runPush(args []string) error {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	zone := fs.String("zone", "", "zone being bootstrapped")
	keyPath := fs.String("key", "", "path to the Ed25519 key (created if missing)")
	record := fs.String("record", "", "record to add, as name=ipv4 (default www.<zone>=203.0.113.10)")
	ttl := fs.Uint("ttl", 300, "TTL for the added record")
	target := fs.String("target", "", "host:port to send the signed push to (omit to just self-verify)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *zone == "" || *keyPath == "" {
		return fmt.Errorf("-zone and -key are required")
	}

	key, priv, generated, err := sazu.LoadOrGenerateKey(*keyPath, *zone, true)
	if err != nil {
		return err
	}
	if generated {
		fmt.Fprintf(os.Stderr, "No key found at %s -- generated a new one.\n", *keyPath)
	}
	printKeyInfo(*keyPath, key)

	rec := *record
	if rec == "" {
		rec = "www." + strings.TrimSuffix(*zone, ".") + "=203.0.113.10"
	}
	name, ipStr, ok := strings.Cut(rec, "=")
	if !ok {
		return fmt.Errorf("-record must be of the form name=ipv4")
	}
	ip := net.ParseIP(ipStr).To4()
	if ip == nil {
		return fmt.Errorf("invalid IPv4 address %q", ipStr)
	}

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(*zone), dns.TypeSOA) // zone section, RFC 2136 §2.3
	m.Opcode = dns.OpcodeUpdate
	// First contact per §10.2: no prerequisites of our own -- the server
	// decides whether a key is already pinned, we just present ourselves.
	m.Insert([]dns.RR{
		&dns.DNSKEY{
			Hdr:       dns.RR_Header{Name: key.Hdr.Name, Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: uint32(*ttl)},
			Flags:     key.Flags,
			Protocol:  key.Protocol,
			Algorithm: key.Algorithm,
			PublicKey: key.PublicKey,
		},
		&dns.A{
			Hdr: dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(*ttl)},
			A:   ip,
		},
	})

	now := time.Now()
	wire, err := sazu.SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		return err
	}
	return signSelfVerifyAndSend(*zone, wire, key, *target)
}

func runPushZone(args []string) error {
	fs := flag.NewFlagSet("push-zone", flag.ExitOnError)
	zone := fs.String("zone", "", "zone being pushed")
	keyPath := fs.String("key", "", "path to the Ed25519 key (created if missing)")
	zoneFile := fs.String("zonefile", "",
		"path to a BIND-format zone file for -zone. Omit this to onboard a brand-new "+
			"domain with nothing pre-authored on disk -- see -ns and -add instead.")
	ns := fs.String("ns", "", "nameserver name for a synthesized SOA/NS (only used without -zonefile; "+
		"defaults to ns1.<zone> if omitted)")
	var adds stringSliceFlag
	fs.Var(&adds, "add", `record to include, zone-file format, e.g. -add "www.example.org. 300 IN A 203.0.113.10" `+
		`(repeatable; only used without -zonefile)`)
	previousSerial := fs.Uint64("previous-serial", 0,
		"SOA serial you last saw published for this zone, to guard against a stale push (RFC 2136 §2.4.2). "+
			"Omit (0) for first contact, where there is nothing yet to be stale against.")
	target := fs.String("target", "", "host:port to send the signed push to (omit to just self-verify)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *zone == "" || *keyPath == "" {
		return fmt.Errorf("-zone and -key are required")
	}

	key, priv, generated, err := sazu.LoadOrGenerateKey(*keyPath, *zone, true)
	if err != nil {
		return err
	}
	if generated {
		fmt.Fprintf(os.Stderr, "No key found at %s -- generated a new one.\n", *keyPath)
	}
	printKeyInfo(*keyPath, key)

	var soa *dns.SOA
	var rrs []dns.RR
	if *zoneFile != "" {
		soa, rrs, err = sazu.LoadZoneFile(*zoneFile, *zone)
		if err != nil {
			return err
		}
		fmt.Printf("Loaded %s: SOA serial %d, %d other record(s)\n", *zoneFile, soa.Serial, len(rrs))
	} else {
		soa = sazu.SynthesizeSOA(*zone, *ns)
		rrs = append(rrs, &dns.NS{
			Hdr: dns.RR_Header{Name: dns.Fqdn(*zone), Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: soa.Hdr.Ttl},
			Ns:  soa.Ns,
		})
		extra, err := parseRRs("-add", adds)
		if err != nil {
			return err
		}
		rrs = append(rrs, extra...)
		fmt.Printf("No -zonefile given -- synthesized SOA serial %d and NS %s, plus %d -add record(s)\n",
			soa.Serial, soa.Ns, len(extra))
	}

	var previousSOA *dns.SOA
	if *previousSerial != 0 {
		prev := *soa
		prev.Serial = uint32(*previousSerial)
		previousSOA = &prev
	}
	m := sazu.BuildFullZonePush(*zone, soa, rrs, key, previousSOA)
	now := time.Now()
	wire, err := sazu.SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		return err
	}
	return signSelfVerifyAndSend(*zone, wire, key, *target)
}

// runPushUpdate builds an ordinary (non-first-contact) SAZU push: no
// DNSKEY add, just RFC 2136 add/delete ops against a zone whose key the
// server has (presumably) already pinned from an earlier push-zone. This
// is what exercises the "partial update" path -- one or a few records
// changing, not a whole zone.
func runPushUpdate(args []string) error {
	fs := flag.NewFlagSet("push-update", flag.ExitOnError)
	zone := fs.String("zone", "", "zone being updated")
	keyPath := fs.String("key", "", "path to the Ed25519 key already pinned at the server for this zone")
	target := fs.String("target", "", "host:port to send the signed push to (omit to just self-verify)")
	var adds, dels, delRRsets stringSliceFlag
	fs.Var(&adds, "add", `record to add, zone-file format, e.g. -add "www.example.org. 300 IN A 203.0.113.20" (repeatable)`)
	fs.Var(&dels, "del", "exact record to delete, same format as -add (repeatable)")
	fs.Var(&delRRsets, "del-rrset", `name and type whose entire RRset should be deleted, e.g. -del-rrset "www.example.org. A" (repeatable)`)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *zone == "" || *keyPath == "" {
		return fmt.Errorf("-zone and -key are required")
	}
	if len(adds) == 0 && len(dels) == 0 && len(delRRsets) == 0 {
		return fmt.Errorf("at least one of -add, -del, or -del-rrset is required")
	}

	key, priv, generated, err := sazu.LoadOrGenerateKey(*keyPath, *zone, true)
	if err != nil {
		return err
	}
	if generated {
		fmt.Fprintf(os.Stderr, "No key found at %s -- generated a new one. A partial update only "+
			"succeeds if the server already pinned this exact key for %s.\n", *keyPath, *zone)
	}
	printKeyInfo(*keyPath, key)

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(*zone), dns.TypeSOA)
	m.Opcode = dns.OpcodeUpdate

	if len(adds) > 0 {
		rrs, err := parseRRs("-add", adds)
		if err != nil {
			return err
		}
		m.Insert(rrs)
	}
	if len(dels) > 0 {
		rrs, err := parseRRs("-del", dels)
		if err != nil {
			return err
		}
		m.Remove(rrs)
	}
	if len(delRRsets) > 0 {
		rrs, err := parseNameTypePairs(delRRsets)
		if err != nil {
			return err
		}
		m.RemoveRRset(rrs)
	}

	now := time.Now()
	wire, err := sazu.SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		return err
	}
	return signSelfVerifyAndSend(*zone, wire, key, *target)
}

// parseRRs parses each s in values as a zone-file-format resource record.
func parseRRs(flagName string, values []string) ([]dns.RR, error) {
	rrs := make([]dns.RR, 0, len(values))
	for _, s := range values {
		rr, err := dns.NewRR(s)
		if err != nil {
			return nil, fmt.Errorf("%s %q: %w", flagName, s, err)
		}
		rrs = append(rrs, rr)
	}
	return rrs, nil
}

// parseNameTypePairs parses each value as "name TYPE" and returns a
// minimal RR of that type carrying only Name/Rrtype/Class -- exactly what
// Msg.RemoveRRset needs, since RFC 2136 §2.5.2 deletes never carry rdata.
func parseNameTypePairs(values []string) ([]dns.RR, error) {
	rrs := make([]dns.RR, 0, len(values))
	for _, s := range values {
		name, typ, ok := strings.Cut(strings.TrimSpace(s), " ")
		if !ok {
			return nil, fmt.Errorf("-del-rrset %q must be \"name TYPE\"", s)
		}
		rtype, ok := dns.StringToType[strings.ToUpper(strings.TrimSpace(typ))]
		if !ok {
			return nil, fmt.Errorf("-del-rrset %q: unknown type %q", s, typ)
		}
		newFn, ok := dns.TypeToRR[rtype]
		if !ok {
			return nil, fmt.Errorf("-del-rrset %q: unsupported type %q", s, typ)
		}
		rr := newFn()
		*rr.Header() = dns.RR_Header{Name: dns.Fqdn(strings.TrimSpace(name)), Rrtype: rtype, Class: dns.ClassINET}
		rrs = append(rrs, rr)
	}
	return rrs, nil
}

// signSelfVerifyAndSend proves a signed push actually verifies against
// its own key before sending anything, then either sends it to target
// over UDP and reports what the server did with it, or just prints the
// wire bytes if no target was given.
func signSelfVerifyAndSend(zone string, wire []byte, key *dns.DNSKEY, target string) error {
	if err := sazu.VerifySIG0(wire, key); err != nil {
		return fmt.Errorf("self-verification failed (this would be a bug): %w", err)
	}
	fmt.Printf("Self-verification: OK (%d bytes)\n", len(wire))

	if target == "" {
		fmt.Printf("No -target given; wire bytes (hex):\n%x\n", wire)
		return nil
	}

	conn, err := net.Dial("udp", target)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Write(wire); err != nil {
		return err
	}
	fmt.Printf("Sent %d bytes to %s\n", len(wire), target)

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("No response (%v) -- fine if nothing is listening yet; "+
			"the push itself encoded, signed, and self-verified correctly.\n", err)
		return nil
	}

	resp := new(dns.Msg)
	if err := resp.Unpack(buf[:n]); err != nil {
		fmt.Printf("Response (%d bytes, did not parse as a DNS message: %v):\n%x\n", n, err, buf[:n])
		return nil
	}
	return interpretResponse(zone, key, resp)
}

// interpretResponse prints a plain-language verdict for the server's
// response to a push, and returns a non-nil error (so sazuctl exits
// non-zero) when the push wasn't accepted. The one case with dedicated
// guidance is ERR_NO_DS_PUBLISHED (§12's status-code convention, carried
// as a TXT record in the response's Additional section) -- by far the
// most common reason a first-contact push gets refused, and the one with
// a concrete, actionable next step.
func interpretResponse(zone string, key *dns.DNSKEY, resp *dns.Msg) error {
	if resp.Rcode == dns.RcodeSuccess {
		fmt.Println("Accepted (NOERROR).")
		return nil
	}

	status, _ := diagnosticStatus(resp)
	if status == statusErrNoDSPublished {
		printNoDSGuidance(zone, key)
		return fmt.Errorf("denied: no DS record published for %s yet", zone)
	}

	rcodeName := dns.RcodeToString[resp.Rcode]
	if status != "" {
		return fmt.Errorf("denied: %s (%s)", rcodeName, status)
	}
	return fmt.Errorf("denied: %s", rcodeName)
}

// statusErrNoDSPublished mirrors the constant of the same name in
// plugin/sazu/handler.go -- kept as a literal here rather than imported
// since it's an unexported implementation detail of the server, not part
// of that package's public API; the wire value is what actually matters,
// and it's fixed by the design doc's §12 status-code list.
const statusErrNoDSPublished = "ERR_NO_DS_PUBLISHED"

// diagnosticStatus extracts a §12 SAZU status code from a response's
// Additional section, if present.
func diagnosticStatus(m *dns.Msg) (string, bool) {
	for _, rr := range m.Extra {
		if txt, ok := rr.(*dns.TXT); ok && len(txt.Txt) > 0 {
			return txt.Txt[0], true
		}
	}
	return "", false
}

func printNoDSGuidance(zone string, key *dns.DNSKEY) {
	ds := key.ToDS(dns.SHA256)
	fmt.Println()
	fmt.Printf("Onboarding denied: no DS record published for %s yet.\n", zone)
	fmt.Println()
	fmt.Println("Your registrar doesn't know about this key. To fix this:")
	fmt.Println()
	fmt.Println("  1. Give your registrar this DS record:")
	fmt.Println()
	fmt.Printf("       %s IN DS %d %d %d %s\n", key.Hdr.Name, ds.KeyTag, ds.Algorithm, ds.DigestType, ds.Digest)
	fmt.Println()
	fmt.Println("     See REGISTRARS.md (plugin/sazu/REGISTRARS.md in this checkout) for")
	fmt.Println("     registrar-specific instructions -- not every registrar is covered yet;")
	fmt.Println("     if yours isn't, search their support site for \"DS record\" or \"DNSSEC.\"")
	fmt.Println()
	fmt.Println("  2. Wait for it to propagate (minutes to a few hours is typical):")
	fmt.Println()
	fmt.Printf("       dig DS %s +short\n", zone)
	fmt.Println()
	fmt.Println("  3. Re-run this same command once that shows your digest.")
	fmt.Println()
}

func printKeyInfo(path string, key *dns.DNSKEY) {
	fmt.Printf("Ed25519 key -> %s\n", path)
	fmt.Printf("  key type:  %d (%s)\n", key.Flags, keyTypeLabel(key.Flags))
	fmt.Printf("  algorithm: %d (%s)\n", key.Algorithm, algorithmLabel(key.Algorithm))
	fmt.Printf("  key tag:   %d\n", key.KeyTag())
	fmt.Printf("  public key (base64): %s\n", key.PublicKey)
}

// keyTypeLabel names the DNSKEY flags value the way registrar UIs
// commonly present it (e.g. AWS Route 53's "public key type" field):
// 256 for a Zone Signing Key (the ZONE bit only) or 257 for a Key Signing
// Key (ZONE + SEP). SAZU always generates SEP-flagged (KSK) keys, so 257
// is what you'll see today, but this stays correct if that ever changes.
func keyTypeLabel(flags uint16) string {
	switch flags {
	case dns.ZONE | dns.SEP:
		return "KSK"
	case dns.ZONE:
		return "ZSK"
	default:
		return "unrecognized flags"
	}
}

func algorithmLabel(algorithm uint8) string {
	if name, ok := dns.AlgorithmToString[algorithm]; ok {
		return name
	}
	return "unknown"
}
