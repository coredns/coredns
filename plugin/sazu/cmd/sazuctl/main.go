// Command sazuctl is a minimal test client for the SAZU push protocol --
// the Go/CoreDNS counterpart to the earlier Rust/rDNS port's sazu-client.
// It builds a first-contact SAZU push (a DNSKEY add for the client's own
// key, plus one A record), signs it with SIG(0) (RFC 2931) using that same
// key -- the design's central decision (§9.1: one key does both jobs) --
// and either sends it to a target (UDP or TCP, chosen automatically by
// size -- see safeUDPPushSize) or just prints/self-verifies it.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/coredns/coredns/plugin/sazu"

	"github.com/miekg/dns"
)

// safeUDPPushSize is the threshold above which sazuctl sends a push over
// TCP instead of UDP: RFC 1035's own original plain-DNS-over-UDP ceiling
// (miekg/dns's MinMsgSize), and -- deliberately -- the real, actual
// receive capacity of a CoreDNS UDP listener today, since core/dnsserver
// does not raise it (an earlier Config.UDPSize override was tried and
// removed; see SAZU-PLAN.md for why). This has to track that real
// capacity exactly, not some larger "should be safe" value: a push
// between 512 bytes and any bigger guess would still go out over UDP,
// still get silently truncated to 512 bytes on receipt, and still fail
// with an unhelpful low-level FORMERR indistinguishable from a genuinely
// malformed request -- a real bug this project hit by picking 1232 (the
// "DNS Flag Day 2020" convention for *response* sizes, which doesn't
// apply here since nothing on this side raises the receive buffer to
// match it). Separately, real DNSSEC-signed content -- this project's
// whole point -- also routinely exceeds the ~1472-byte path MTU and gets
// fragmented at the IP layer, which many real firewalls and security
// groups silently drop entirely; TCP avoids that failure mode too, for
// the same reason RFC 1035 built it in as DNS's fallback transport from
// the very beginning, later formalized as a requirement in RFC 7766.
// There is no "split one UPDATE across several UDP datagrams" mechanism
// in RFC 2136 or any real implementation, so escalating transport, not
// shrinking the message, is the only real option once a push exceeds
// either ceiling.
const safeUDPPushSize = 512

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
	case "contact":
		err = runContact(os.Args[2:])
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
	fmt.Fprintln(os.Stderr, "  sazuctl keygen -out <path> [-zone <owner>] [-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl ds -zone <zone> -key <path> [-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl push -zone <zone> -key <path> [-record name=ipv4] [-ttl 300] [-target host:port] [-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl push-zone -zone <zone> -key <path> -zonefile <path> [-previous-serial N] [-target host:port] [-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl push-update -zone <zone> -key <path> [-add \"rr\"]... [-del \"rr\"]... [-del-rrset \"name TYPE\"]... [-target host:port] [-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl contact -zone <zone> -key <path> [-address mailto:you@example.org]... [-clear] [-target host:port] [-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "-key-passphrase-file encrypts/decrypts the key file at rest (§10.8); omit it for a plain BIND-format key file (the default).")
}

// addPassphraseFlag registers the -key-passphrase-file flag every
// subcommand that touches a private key file shares: §10.8 key custody
// hardening is opt-in and uniform across all of them -- give this flag
// and the key file is read/written encrypted (see
// sazu.SaveEncryptedPrivateKey), omit it and behavior is unchanged from
// before this existed (a plain BIND-format file).
func addPassphraseFlag(fs *flag.FlagSet) *string {
	return fs.String("key-passphrase-file", "",
		"path to a file whose contents (trimmed of a trailing newline) are the passphrase to "+
			"encrypt/decrypt -key/-out with. Omit for a plain, unencrypted key file (the default).")
}

// readPassphraseFile reads the passphrase addPassphraseFlag's flag points
// at, or returns nil (meaning "unencrypted") if path is empty.
func readPassphraseFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading -key-passphrase-file: %w", err)
	}
	data = bytes.TrimRight(data, "\r\n")
	if len(data) == 0 {
		return nil, fmt.Errorf("-key-passphrase-file %s is empty", path)
	}
	return data, nil
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
	passphraseFile := addPassphraseFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("-out is required")
	}
	passphrase, err := readPassphraseFile(*passphraseFile)
	if err != nil {
		return err
	}

	key, priv, err := sazu.GenerateEd25519Key(*zone, true)
	if err != nil {
		return err
	}
	if passphrase != nil {
		err = sazu.SaveEncryptedPrivateKey(*out, key, priv, passphrase)
	} else {
		err = sazu.SavePrivateKey(*out, key, priv)
	}
	if err != nil {
		return err
	}
	printKeyInfo(*out, key)
	if passphrase != nil {
		fmt.Println("(encrypted at rest with the given passphrase)")
	}
	return nil
}

func runDS(args []string) error {
	fs := flag.NewFlagSet("ds", flag.ExitOnError)
	zone := fs.String("zone", "", "zone this key is for")
	keyPath := fs.String("key", "", "path to the Ed25519 key (created if missing)")
	passphraseFile := addPassphraseFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *zone == "" || *keyPath == "" {
		return fmt.Errorf("-zone and -key are required")
	}
	passphrase, err := readPassphraseFile(*passphraseFile)
	if err != nil {
		return err
	}

	key, _, generated, err := sazu.LoadOrGenerateKey(*keyPath, *zone, true, passphrase)
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
	passphraseFile := addPassphraseFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *zone == "" || *keyPath == "" {
		return fmt.Errorf("-zone and -key are required")
	}
	passphrase, err := readPassphraseFile(*passphraseFile)
	if err != nil {
		return err
	}

	key, priv, generated, err := sazu.LoadOrGenerateKey(*keyPath, *zone, true, passphrase)
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
	zoneFile := fs.String("zonefile", "", "path to a BIND-format zone file for -zone")
	previousSerial := fs.Uint64("previous-serial", 0,
		"SOA serial you last saw published for this zone, to guard against a stale push (RFC 2136 §2.4.2). "+
			"Omit (0) for first contact, where there is nothing yet to be stale against.")
	target := fs.String("target", "", "host:port to send the signed push to (omit to just self-verify)")
	passphraseFile := addPassphraseFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *zone == "" || *keyPath == "" || *zoneFile == "" {
		return fmt.Errorf("-zone, -key, and -zonefile are required")
	}
	passphrase, err := readPassphraseFile(*passphraseFile)
	if err != nil {
		return err
	}

	key, priv, generated, err := sazu.LoadOrGenerateKey(*keyPath, *zone, true, passphrase)
	if err != nil {
		return err
	}
	if generated {
		fmt.Fprintf(os.Stderr, "No key found at %s -- generated a new one.\n", *keyPath)
	}
	printKeyInfo(*keyPath, key)

	soa, rrs, err := sazu.LoadZoneFile(*zoneFile, *zone)
	if err != nil {
		return err
	}
	fmt.Printf("Loaded %s: SOA serial %d, %d other record(s)\n", *zoneFile, soa.Serial, len(rrs))

	var previousSOA *dns.SOA
	if *previousSerial != 0 {
		prev := *soa
		prev.Serial = uint32(*previousSerial)
		previousSOA = &prev
	}
	m, err := sazu.BuildFullZonePush(*zone, soa, rrs, key, priv, previousSOA)
	if err != nil {
		return err
	}
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
	passphraseFile := addPassphraseFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *zone == "" || *keyPath == "" {
		return fmt.Errorf("-zone and -key are required")
	}
	if len(adds) == 0 && len(dels) == 0 && len(delRRsets) == 0 {
		return fmt.Errorf("at least one of -add, -del, or -del-rrset is required")
	}
	passphrase, err := readPassphraseFile(*passphraseFile)
	if err != nil {
		return err
	}

	key, priv, generated, err := sazu.LoadOrGenerateKey(*keyPath, *zone, true, passphrase)
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
		now := time.Now()
		signed, err := sazu.SignZoneContent(rrs, key, priv, now.Add(-sazu.DefaultSignatureInceptionSkew), now.Add(sazu.DefaultSignatureValidity))
		if err != nil {
			return fmt.Errorf("signing added records: %w", err)
		}
		m.Insert(signed)
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

// runContact registers or clears a zone's §10.6 registration-contact
// address(es) -- the address(es) sazu-watchd (§11) alerts on delegation
// changes. A separate subcommand from push-update, rather than telling
// users to reach for -add themselves, specifically so nobody accidentally
// runs the contact TXT through the zone-content signing path (see
// sazu.BuildContactOp's doc comment for why that would silently do the
// wrong thing).
func runContact(args []string) error {
	fs := flag.NewFlagSet("contact", flag.ExitOnError)
	zone := fs.String("zone", "", "zone to register a contact for")
	keyPath := fs.String("key", "", "path to the Ed25519 key already pinned at the server for this zone")
	target := fs.String("target", "", "host:port to send the signed push to (omit to just self-verify)")
	clear := fs.Bool("clear", false, "clear the zone's registered contact instead of setting one")
	var addresses stringSliceFlag
	fs.Var(&addresses, "address", "contact address: mailto:you@example.org, or https://... for a webhook (repeatable)")
	passphraseFile := addPassphraseFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *zone == "" || *keyPath == "" {
		return fmt.Errorf("-zone and -key are required")
	}
	if *clear == (len(addresses) > 0) {
		return fmt.Errorf("specify exactly one of -clear or one or more -address")
	}
	passphrase, err := readPassphraseFile(*passphraseFile)
	if err != nil {
		return err
	}

	key, priv, generated, err := sazu.LoadOrGenerateKey(*keyPath, *zone, true, passphrase)
	if err != nil {
		return err
	}
	if generated {
		fmt.Fprintf(os.Stderr, "No key found at %s -- generated a new one. This only succeeds if the "+
			"server already pinned this exact key for %s.\n", *keyPath, *zone)
	}
	printKeyInfo(*keyPath, key)

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(*zone), dns.TypeSOA)
	m.Opcode = dns.OpcodeUpdate

	if *clear {
		m.Remove([]dns.RR{&dns.TXT{Hdr: dns.RR_Header{Name: sazu.ContactOwnerName(*zone), Rrtype: dns.TypeTXT, Class: dns.ClassINET}}})
	} else {
		op, err := sazu.BuildContactOp(*zone, addresses)
		if err != nil {
			return err
		}
		m.Insert([]dns.RR{op})
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

	network := "udp"
	if len(wire) > safeUDPPushSize {
		network = "tcp"
	}

	conn, err := net.Dial(network, target)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	if err := writeRequest(conn, network, wire); err != nil {
		return err
	}
	if network == "tcp" {
		fmt.Printf("Sent %d bytes to %s over TCP (exceeds the %d-byte safe UDP size)\n", len(wire), target, safeUDPPushSize)
	} else {
		fmt.Printf("Sent %d bytes to %s\n", len(wire), target)
	}

	buf, err := readResponse(conn, network)
	if err != nil {
		fmt.Printf("No response (%v) -- fine if nothing is listening yet; "+
			"the push itself encoded, signed, and self-verified correctly.\n", err)
		return nil
	}

	resp := new(dns.Msg)
	if err := resp.Unpack(buf); err != nil {
		fmt.Printf("Response (%d bytes, did not parse as a DNS message: %v):\n%x\n", len(buf), err, buf)
		return nil
	}
	return interpretResponse(zone, key, resp)
}

// writeRequest sends wire to conn, prefixing it with the 2-byte
// big-endian length RFC 1035 §4.2.2 requires for TCP framing (not needed
// for UDP, which is message-oriented already).
func writeRequest(conn net.Conn, network string, wire []byte) error {
	if network == "tcp" {
		var lenPrefix [2]byte
		binary.BigEndian.PutUint16(lenPrefix[:], uint16(len(wire)))
		if _, err := conn.Write(lenPrefix[:]); err != nil {
			return err
		}
	}
	_, err := conn.Write(wire)
	return err
}

// readResponse reads one complete response message from conn, handling
// TCP's length-prefix framing.
func readResponse(conn net.Conn, network string) ([]byte, error) {
	if network == "tcp" {
		var lenPrefix [2]byte
		if _, err := io.ReadFull(conn, lenPrefix[:]); err != nil {
			return nil, err
		}
		buf := make([]byte, binary.BigEndian.Uint16(lenPrefix[:]))
		if _, err := io.ReadFull(conn, buf); err != nil {
			return nil, err
		}
		return buf, nil
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
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
	switch status {
	case statusErrNoDSPublished:
		printNoDSGuidance(zone, key)
		return fmt.Errorf("denied: no DS record published for %s yet", zone)
	case statusErrUnknownSigner:
		printUnknownSignerGuidance(zone, key)
		return fmt.Errorf("denied: a DS record for %s is already published, but not for this key", zone)
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

// statusErrUnknownSigner mirrors the constant of the same name in
// plugin/sazu/handler.go, for the same reason statusErrNoDSPublished does.
const statusErrUnknownSigner = "ERR_UNKNOWN_SIGNER"

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
	fmt.Println("  0. If this domain is CURRENTLY LIVE and serving real traffic, and its")
	fmt.Println("     current host has never published DNSSEC for it before (which is why")
	fmt.Println("     you're seeing this message), read this first:")
	fmt.Println()
	fmt.Println("     Publishing the DS record below makes every DNSSEC-validating resolver")
	fmt.Println("     in the world expect signed answers for this domain immediately -- not")
	fmt.Println("     only once this server is actually authoritative for it. Until the")
	fmt.Println("     cutover to this server is complete, the domain's *current* host is")
	fmt.Println("     still the one answering, and if it isn't serving matching signatures,")
	fmt.Println("     the entire domain (not just DNSSEC-specific lookups) breaks --")
	fmt.Println("     SERVFAIL for every validating resolver -- for as long as that mismatch")
	fmt.Println("     lasts. A brand-new domain with no live traffic yet has no such risk.")
	fmt.Println()
	fmt.Println("     If your current host supports enabling its own DNSSEC signing, turn")
	fmt.Println("     that on there FIRST and confirm the domain still resolves correctly")
	fmt.Println("     everywhere -- that keeps it validly signed throughout the migration,")
	fmt.Println("     under its own key, right up until you actually cut over to this")
	fmt.Println("     server (at which point its DS gets replaced by the one below). If")
	fmt.Println("     your current host has no way to enable DNSSEC at all, consider moving")
	fmt.Println("     this domain's hosting to one that does (e.g. AWS Route 53) before")
	fmt.Println("     publishing anything below.")
	fmt.Println()
	fmt.Println("  1. Give your registrar this DS record:")
	fmt.Println()
	fmt.Printf("       %s IN DS %d %d %d %s\n", key.Hdr.Name, ds.KeyTag, ds.Algorithm, ds.DigestType, ds.Digest)
	fmt.Println()
	printRegistrarKeyFieldsAlternative(key)
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

// printUnknownSignerGuidance explains ERR_UNKNOWN_SIGNER: a DS record
// already exists for zone, just not for this key. Deliberately does not
// assume anything adversarial -- the far more likely explanation is that
// the zone's current host already has its own DNSSEC set up (its own
// key, unrelated to SAZU), which is exactly the state the no-DS guidance
// above recommends putting a domain into during migration. Unlike an
// earlier version of this message, this gives a concrete way to actually
// get onboarded now rather than just "investigate and wait": chain.go's
// VerifyChainOfTrust accepts a candidate key as soon as *any* published DS
// matches it, so a second, coexisting DS record for this key is enough --
// nothing needs to be removed first.
func printUnknownSignerGuidance(zone string, key *dns.DNSKEY) {
	ds := key.ToDS(dns.SHA256)
	fmt.Println()
	fmt.Printf("Onboarding denied: a DS record is already published for %s, but not for\n", zone)
	fmt.Println("this key.")
	fmt.Println()
	fmt.Println("This is not necessarily a problem with this key, and not necessarily an")
	fmt.Println("attack -- it most likely means DNSSEC is already enabled for this domain")
	fmt.Println("under a different key, quite possibly its current host's own (e.g. if you")
	fmt.Println("followed the migration guidance to enable DNSSEC there first). Check what's")
	fmt.Println("actually publishing that DS before doing anything about it:")
	fmt.Println()
	fmt.Printf("  dig DS %s +short\n", zone)
	fmt.Println()
	fmt.Println("You can still get this key onboarded now, without disturbing that one. Most")
	fmt.Println("registrars accept more than one DS record for the same domain at once --")
	fmt.Println("this is exactly how a DNSSEC key or algorithm rollover works (RFC 6781")
	fmt.Println("§4.1.4) -- so:")
	fmt.Println()
	fmt.Println("  1. Give your registrar this DS record IN ADDITION TO the one already")
	fmt.Println("     there -- do not remove or replace the existing one yet:")
	fmt.Println()
	fmt.Printf("       %s IN DS %d %d %d %s\n", key.Hdr.Name, ds.KeyTag, ds.Algorithm, ds.DigestType, ds.Digest)
	fmt.Println()
	printRegistrarKeyFieldsAlternative(key)
	fmt.Println("     See REGISTRARS.md (plugin/sazu/REGISTRARS.md in this checkout) for")
	fmt.Println("     registrar-specific instructions, including how to add a second DS/key")
	fmt.Println("     record rather than replacing the one already there.")
	fmt.Println()
	fmt.Println("  2. Wait for it to propagate, then confirm both digests are visible:")
	fmt.Println()
	fmt.Printf("       dig DS %s +short\n", zone)
	fmt.Println()
	fmt.Println("  3. Re-run this same command once that shows both. Onboarding succeeds as")
	fmt.Println("     soon as this key's own DS is visible -- the other, coexisting DS")
	fmt.Println("     record doesn't block it.")
	fmt.Println()
	fmt.Println("  4. Only once you are actually ready to cut authoritative service over to")
	fmt.Println("     this server, remove the OTHER DS record (the one that was already")
	fmt.Println("     there before this key's). Leaving it in place until then is harmless:")
	fmt.Println("     this server only cares that its own key's DS is among whatever is")
	fmt.Println("     published, not that it's the only one.")
	fmt.Println()
	fmt.Println("If your registrar's DNSSEC panel only accepts a single DS record and won't")
	fmt.Println("let you add a second one, you can't do the above safely: replacing the sole")
	fmt.Println("DS record before you're ready for cutover breaks the domain the same way")
	fmt.Println("publishing a DS too early does on a domain with no DNSSEC at all (see the")
	fmt.Println("no-DS-published guidance for why). In that case, wait until cutover, then")
	fmt.Println("replace the existing DS with this one at the same time you switch")
	fmt.Println("delegation to this server.")
	fmt.Println()
	fmt.Println("If you don't recognize the existing DS at all -- it isn't your current")
	fmt.Println("host's own DNSSEC and nothing you set up -- treat it as a real incident:")
	fmt.Println("stop and investigate with your registrar before proceeding.")
	fmt.Println()
}

// printRegistrarKeyFieldsAlternative prints the DNSKEY's own fields
// (public key type, algorithm, key tag, public key) as an alternative to
// the DS record printed just above it. Not every registrar's DNSSEC UI
// takes a DS record directly -- AWS Route 53 is a confirmed example (see
// REGISTRARS.md) of one that instead asks you to enter these fields
// yourself and computes the DS itself, which needs manual, one-field-at-
// a-time entry rather than pasting a single string.
func printRegistrarKeyFieldsAlternative(key *dns.DNSKEY) {
	fmt.Println("     Some registrars (e.g. AWS Route 53) instead ask you to enter the key's")
	fmt.Println("     own fields by hand and compute the DS themselves. If that's what you're")
	fmt.Println("     looking at, enter:")
	fmt.Println()
	fmt.Printf("       public key type: %d (%s)\n", key.Flags, keyTypeLabel(key.Flags))
	fmt.Printf("       algorithm:       %d (%s)\n", key.Algorithm, algorithmLabel(key.Algorithm))
	fmt.Printf("       key tag:         %d\n", key.KeyTag())
	fmt.Printf("       public key (base64): %s\n", key.PublicKey)
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
