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
	"crypto/ed25519"
	"embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/coredns/coredns/plugin/pkg/doh"
	"github.com/coredns/coredns/plugin/sazu"

	"github.com/miekg/dns"
)

// guidanceFS embeds every long, user-facing explanatory text this tool
// prints -- the onboarding-denied diagnostics and the KSK/ZSK rotation
// decision below -- as separate template files under guidance/, rather
// than as long chains of fmt.Println calls in this source file. They are
// still fully compiled into the sazuctl binary (go:embed copies their
// contents into the binary at build time, so nothing needs to ship or be
// installed alongside it) -- separating them out just keeps this file's
// actual logic legible, and keeps this occasionally quite long prose
// editable on its own, independent of the code that decides when to show
// it. See guidance/*.txt for the actual wording.
//
//go:embed guidance/*.txt
var guidanceFS embed.FS

// guidanceTemplates is parsed once at startup; ExecuteTemplate below
// just selects which named template (one per file, declared via that
// file's own {{define "name"}}) to render for a given situation.
// template.Must panics if a guidance file has a syntax error -- exactly
// the outcome wanted for a bug that would otherwise silently ship a
// blank or truncated message to a real customer.
var guidanceTemplates = template.Must(template.ParseFS(guidanceFS, "guidance/*.txt"))

// printGuidance renders the named embedded template to stdout. A
// rendering error here is a bug in a guidance file, not a runtime
// condition this tool's own users can hit or need to react to, so it's
// reported plainly to stderr rather than treated as a command failure.
func printGuidance(name string, data any) {
	if err := guidanceTemplates.ExecuteTemplate(os.Stdout, name, data); err != nil {
		fmt.Fprintf(os.Stderr, "sazuctl: internal error rendering guidance %q: %v\n", name, err)
	}
}

// dsGuidanceData is the template data shared by no-ds.txt,
// unknown-signer.txt, and registrar-key-fields.txt (the DS/key-fields
// block those two both include).
type dsGuidanceData struct {
	Zone          string
	Owner         string
	KeyTag        uint16
	Algorithm     uint8
	AlgorithmName string
	DigestType    uint8
	Digest        string
	KeyTypeValue  uint16
	KeyTypeLabel  string
	PublicKeyB64  string
}

func dsGuidanceDataFor(zone string, key *dns.DNSKEY) dsGuidanceData {
	ds := key.ToDS(dns.SHA256)
	return dsGuidanceData{
		Zone: zone, Owner: key.Hdr.Name,
		KeyTag: ds.KeyTag, Algorithm: ds.Algorithm, AlgorithmName: algorithmLabel(key.Algorithm),
		DigestType: ds.DigestType, Digest: ds.Digest,
		KeyTypeValue: key.Flags, KeyTypeLabel: keyTypeLabel(key.Flags),
		PublicKeyB64: key.PublicKey,
	}
}

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
	case "add-zsk":
		err = runAddZSK(os.Args[2:])
	case "retire-zsk":
		err = runRetireZSK(os.Args[2:])
	case "rotate-key":
		err = runRotateKey(os.Args[2:])
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
	fmt.Fprintln(os.Stderr, "usage: sazuctl <keygen|ds|push|push-zone|push-update|contact|add-zsk|retire-zsk|rotate-key> [flags]")
	fmt.Fprintln(os.Stderr, "  sazuctl keygen -out <path> [-zone <owner>] [-role ksk|zsk] [-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl ds -zone <zone> -key <path> [-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl push -zone <zone> -key <path> [-record name=ipv4] [-ttl 300] [-target host:port|url] [-json] [-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl push-zone -zone <zone> -key <path> -zonefile <path> [-zsk-key <path>] [-previous-serial N] [-target host:port|url] [-json] [-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl push-update -zone <zone> -key <path> [-zsk-key <path>] [-add \"rr\"]... [-del \"rr\"]... [-del-rrset \"name TYPE\"]... [-target host:port|url] [-json] [-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl contact -zone <zone> -key <path> [-address mailto:you@example.org]... [-clear] [-target host:port|url] [-json] [-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl add-zsk -zone <zone> -ksk-key <path> -zsk-key <path> [-target host:port|url] [-json] [-key-passphrase-file <path>] [-zsk-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl retire-zsk -zone <zone> -ksk-key <path> -zsk-key <path> [-target host:port|url] [-json] [-key-passphrase-file <path>] [-zsk-key-passphrase-file <path>]")
	fmt.Fprintln(os.Stderr, "  sazuctl rotate-key -zone <zone> [-role ksk|zsk] ... (run with no -role for an explanation of the choice)")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "-key-passphrase-file encrypts/decrypts the key file at rest (§10.8); omit it for a plain BIND-format key file (the default).")
	fmt.Fprintln(os.Stderr, "-target accepts an http(s):// URL to push over §7.3's HTTPS carrier instead of UDP/TCP; -json then sends a JSON wire envelope instead of raw bytes.")
	fmt.Fprintln(os.Stderr, "-zsk-key, where accepted, signs zone content with that optional ZSK instead of -key (the KSK); -key still authenticates the transaction. See 'sazuctl rotate-key' for the KSK-vs-ZSK tradeoff.")
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

// addJSONCarrierFlag registers the -json flag every push-capable
// subcommand shares: §7.3's HTTPS/JSON carrier, meaningful only when
// -target is an http(s):// URL (see signSelfVerifyAndSend). Off by
// default -- a raw application/dns-message POST body (the RFC 8484 DoH
// convention this project's HTTPS carrier reuses as-is) is the simpler,
// smaller default; -json switches to the {"wire": "<base64>"} envelope
// for a deployment that specifically wants JSON instead.
func addJSONCarrierFlag(fs *flag.FlagSet) *bool {
	return fs.Bool("json", false,
		"when -target is an http(s):// URL, send the push as a JSON wire envelope "+
			`({"wire":"<base64>"}) instead of a raw application/dns-message body`)
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

// addRoleFlag registers the -role flag every subcommand that generates
// or identifies a key by its DNSSEC role shares. Defaults to "ksk" --
// every zone needs exactly one of those and it's what every pre-ZSK
// version of this tool always generated, so a caller that never passes
// this flag sees no behavior change. See keys.go's KeyRole doc comment
// (plugin/sazu) for what the optional "zsk" role is for.
func addRoleFlag(fs *flag.FlagSet) *string {
	return fs.String("role", "ksk", `key role: "ksk" (default -- every zone needs exactly one) or "zsk" (optional, see 'sazuctl rotate-key')`)
}

func parseRoleFlag(role string) (ksk bool, err error) {
	switch role {
	case "ksk":
		return true, nil
	case "zsk":
		return false, nil
	default:
		return false, fmt.Errorf(`-role must be "ksk" or "zsk", got %q`, role)
	}
}

func runKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", "", "path to write the new key to")
	zone := fs.String("zone", "example.org", "owner name for the key (cosmetic until push)")
	role := addRoleFlag(fs)
	passphraseFile := addPassphraseFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("-out is required")
	}
	ksk, err := parseRoleFlag(*role)
	if err != nil {
		return err
	}
	passphrase, err := readPassphraseFile(*passphraseFile)
	if err != nil {
		return err
	}

	key, priv, err := sazu.GenerateEd25519Key(*zone, ksk)
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
	target := fs.String("target", "", "host:port, or an http(s):// URL for the §7.3 HTTPS carrier, to send the signed push to (omit to just self-verify)")
	jsonCarrier := addJSONCarrierFlag(fs)
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
	return signSelfVerifyAndSend(*zone, wire, key, *target, *jsonCarrier)
}

// addZSKKeyFlag registers the -zsk-key (and matching -zsk-key-
// passphrase-file) flag every content-pushing subcommand that can
// optionally sign with a registered ZSK instead of the KSK shares. Empty
// (the default) means "sign content with -key itself, exactly as this
// tool always has" -- the ZSK split is entirely opt-in; see keys.go's
// KeyRole doc comment (plugin/sazu) for what it's for.
func addZSKKeyFlag(fs *flag.FlagSet) (path, passphraseFile *string) {
	path = fs.String("zsk-key", "",
		"path to an existing, already-registered ZSK to sign zone content with instead of -key "+
			"(optional -- omit to sign with -key/the KSK, as always). Never generated automatically: "+
			"register one first with 'sazuctl add-zsk'.")
	passphraseFile = fs.String("zsk-key-passphrase-file", "", "like -key-passphrase-file, but for -zsk-key")
	return path, passphraseFile
}

// loadOptionalZSK loads the key -zsk-key names, if given, erroring
// clearly (never auto-generating -- see addZSKKeyFlag) if the path
// doesn't exist. Returns nil, nil, nil if path is empty.
func loadOptionalZSK(path, passphraseFilePath, zone string) (*dns.DNSKEY, ed25519.PrivateKey, error) {
	if path == "" {
		return nil, nil, nil
	}
	passphrase, err := readPassphraseFile(passphraseFilePath)
	if err != nil {
		return nil, nil, err
	}
	priv, err := sazu.LoadPrivateKey(path, passphrase)
	if err != nil {
		return nil, nil, fmt.Errorf("-zsk-key %s: %w (a ZSK must already be registered with 'sazuctl add-zsk' -- this flag never generates one)", path, err)
	}
	return sazu.DNSKEYFor(zone, priv, false), priv, nil
}

func runPushZone(args []string) error {
	fs := flag.NewFlagSet("push-zone", flag.ExitOnError)
	zone := fs.String("zone", "", "zone being pushed")
	keyPath := fs.String("key", "", "path to the Ed25519 KSK (created if missing)")
	zoneFile := fs.String("zonefile", "", "path to a BIND-format zone file for -zone")
	zskKeyPath, zskPassphraseFile := addZSKKeyFlag(fs)
	previousSerial := fs.Uint64("previous-serial", 0,
		"SOA serial you last saw published for this zone, to guard against a stale push (RFC 2136 §2.4.2). "+
			"Omit (0) for first contact, where there is nothing yet to be stale against.")
	target := fs.String("target", "", "host:port, or an http(s):// URL for the §7.3 HTTPS carrier, to send the signed push to (omit to just self-verify)")
	jsonCarrier := addJSONCarrierFlag(fs)
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

	zsk, zskPriv, err := loadOptionalZSK(*zskKeyPath, *zskPassphraseFile, *zone)
	if err != nil {
		return err
	}
	if zsk != nil {
		printKeyInfo(*zskKeyPath, zsk)
		fmt.Println("(signing zone content with this ZSK; the KSK above only authenticates the transaction and signs the DNSKEY set)")
	}

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
	m, err := sazu.BuildFullZonePushSplit(*zone, soa, rrs, key, priv, zsk, zskPriv, previousSOA)
	if err != nil {
		return err
	}
	now := time.Now()
	wire, err := sazu.SignUpdate(m, key, priv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		return err
	}
	return signSelfVerifyAndSend(*zone, wire, key, *target, *jsonCarrier)
}

// runPushUpdate builds an ordinary (non-first-contact) SAZU push: no
// DNSKEY add, just RFC 2136 add/delete ops against a zone whose key the
// server has (presumably) already pinned from an earlier push-zone. This
// is what exercises the "partial update" path -- one or a few records
// changing, not a whole zone.
func runPushUpdate(args []string) error {
	fs := flag.NewFlagSet("push-update", flag.ExitOnError)
	zone := fs.String("zone", "", "zone being updated")
	keyPath := fs.String("key", "", "path to the Ed25519 key already trusted to authenticate a transaction for this zone (the KSK, or a registered ZSK)")
	zskKeyPath, zskPassphraseFile := addZSKKeyFlag(fs)
	target := fs.String("target", "", "host:port, or an http(s):// URL for the §7.3 HTTPS carrier, to send the signed push to (omit to just self-verify)")
	jsonCarrier := addJSONCarrierFlag(fs)
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

	contentKey, contentPriv := key, priv
	zsk, zskPriv, err := loadOptionalZSK(*zskKeyPath, *zskPassphraseFile, *zone)
	if err != nil {
		return err
	}
	if zsk != nil {
		printKeyInfo(*zskKeyPath, zsk)
		fmt.Println("(signing added content with this ZSK; -key only authenticates the transaction)")
		contentKey, contentPriv = zsk, zskPriv
	}

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(*zone), dns.TypeSOA)
	m.Opcode = dns.OpcodeUpdate

	if len(adds) > 0 {
		rrs, err := parseRRs("-add", adds)
		if err != nil {
			return err
		}
		now := time.Now()
		signed, err := sazu.SignZoneContent(rrs, contentKey, contentPriv, now.Add(-sazu.DefaultSignatureInceptionSkew), now.Add(sazu.DefaultSignatureValidity))
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
	return signSelfVerifyAndSend(*zone, wire, key, *target, *jsonCarrier)
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
	target := fs.String("target", "", "host:port, or an http(s):// URL for the §7.3 HTTPS carrier, to send the signed push to (omit to just self-verify)")
	jsonCarrier := addJSONCarrierFlag(fs)
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
	return signSelfVerifyAndSend(*zone, wire, key, *target, *jsonCarrier)
}

// runAddZSK registers a new, optional ZSK for a zone that already has a
// KSK -- the cheap path (see keys.go's KeyRole doc comment,
// plugin/sazu): an ordinary push, authenticated by -ksk-key (the KSK, or
// any key already trusted to authenticate a transaction for this zone),
// that adds -zsk-key's DNSKEY record. No registrar interaction, no
// chain-of-trust network walk on the server's side.
func runAddZSK(args []string) error {
	fs := flag.NewFlagSet("add-zsk", flag.ExitOnError)
	zone := fs.String("zone", "", "zone to register a new ZSK for")
	kskPath := fs.String("ksk-key", "", "path to a key already trusted to authenticate a transaction for this zone (ordinarily the KSK)")
	zskPath := fs.String("zsk-key", "", "path to the ZSK to register (created if missing)")
	target := fs.String("target", "", "host:port, or an http(s):// URL for the §7.3 HTTPS carrier, to send the signed push to (omit to just self-verify)")
	jsonCarrier := addJSONCarrierFlag(fs)
	kskPassphraseFile := addPassphraseFlag(fs)
	zskPassphraseFile := fs.String("zsk-key-passphrase-file", "", "like -key-passphrase-file, but for -zsk-key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *zone == "" || *kskPath == "" || *zskPath == "" {
		return fmt.Errorf("-zone, -ksk-key, and -zsk-key are required")
	}
	kskPassphrase, err := readPassphraseFile(*kskPassphraseFile)
	if err != nil {
		return err
	}
	ksk, kskPriv, _, err := sazu.LoadOrGenerateKey(*kskPath, *zone, true, kskPassphrase)
	if err != nil {
		return err
	}
	printKeyInfo(*kskPath, ksk)

	zskPassphrase, err := readPassphraseFile(*zskPassphraseFile)
	if err != nil {
		return err
	}
	zsk, _, generated, err := sazu.LoadOrGenerateKey(*zskPath, *zone, false, zskPassphrase)
	if err != nil {
		return err
	}
	if generated {
		fmt.Fprintf(os.Stderr, "No ZSK found at %s -- generated a new one.\n", *zskPath)
	}
	printKeyInfo(*zskPath, zsk)

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(*zone), dns.TypeSOA)
	m.Opcode = dns.OpcodeUpdate
	m.Insert([]dns.RR{&dns.DNSKEY{
		Hdr:   dns.RR_Header{Name: dns.Fqdn(*zone), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags: zsk.Flags, Protocol: zsk.Protocol, Algorithm: zsk.Algorithm, PublicKey: zsk.PublicKey,
	}})

	now := time.Now()
	wire, err := sazu.SignUpdate(m, ksk, kskPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		return err
	}
	fmt.Printf("Registering ZSK key tag %d for %s, authenticated by key tag %d\n", zsk.KeyTag(), *zone, ksk.KeyTag())
	return signSelfVerifyAndSend(*zone, wire, ksk, *target, *jsonCarrier)
}

// runRetireZSK removes a previously registered ZSK from a zone -- the
// RFC 2136 §2.5.4 "delete one RR" shape findRetiredZSKKeytag
// (plugin/sazu/handler.go) looks for. -zsk-key must already exist (it is
// never generated here -- retiring a key that was never created makes no
// sense).
func runRetireZSK(args []string) error {
	fs := flag.NewFlagSet("retire-zsk", flag.ExitOnError)
	zone := fs.String("zone", "", "zone to retire a ZSK from")
	kskPath := fs.String("ksk-key", "", "path to a key already trusted to authenticate a transaction for this zone (ordinarily the KSK)")
	zskPath := fs.String("zsk-key", "", "path to the ZSK being retired (must already exist)")
	target := fs.String("target", "", "host:port, or an http(s):// URL for the §7.3 HTTPS carrier, to send the signed push to (omit to just self-verify)")
	jsonCarrier := addJSONCarrierFlag(fs)
	kskPassphraseFile := addPassphraseFlag(fs)
	zskPassphraseFile := fs.String("zsk-key-passphrase-file", "", "like -key-passphrase-file, but for -zsk-key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *zone == "" || *kskPath == "" || *zskPath == "" {
		return fmt.Errorf("-zone, -ksk-key, and -zsk-key are required")
	}
	kskPassphrase, err := readPassphraseFile(*kskPassphraseFile)
	if err != nil {
		return err
	}
	ksk, kskPriv, _, err := sazu.LoadOrGenerateKey(*kskPath, *zone, true, kskPassphrase)
	if err != nil {
		return err
	}
	printKeyInfo(*kskPath, ksk)

	zsk, _, err := loadOptionalZSK(*zskPath, *zskPassphraseFile, *zone)
	if err != nil {
		return err
	}
	printKeyInfo(*zskPath, zsk)

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(*zone), dns.TypeSOA)
	m.Opcode = dns.OpcodeUpdate
	m.Remove([]dns.RR{&dns.DNSKEY{
		Hdr:   dns.RR_Header{Name: dns.Fqdn(*zone), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags: zsk.Flags, Protocol: zsk.Protocol, Algorithm: zsk.Algorithm, PublicKey: zsk.PublicKey,
	}})

	now := time.Now()
	wire, err := sazu.SignUpdate(m, ksk, kskPriv, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		return err
	}
	fmt.Printf("Retiring ZSK key tag %d for %s, authenticated by key tag %d\n", zsk.KeyTag(), *zone, ksk.KeyTag())
	return signSelfVerifyAndSend(*zone, wire, ksk, *target, *jsonCarrier)
}

// rotateKeyChoiceData is rotate-key-choice.txt's template data.
type rotateKeyChoiceData struct {
	Zone      string
	HasZSK    bool
	ZSKKeyTag uint16
}

// runRotateKey is the decision-support entry point a customer reaches
// for whenever they want to rotate *some* key and isn't sure which kind
// -- the ZSK/KSK tradeoff this whole feature is about. Run with no -role
// at all, it makes no change and instead prints rotate-key-choice.txt
// explaining the tradeoff and asking the user to choose explicitly; it
// only ever acts once -role is given.
func runRotateKey(args []string) error {
	fs := flag.NewFlagSet("rotate-key", flag.ExitOnError)
	zone := fs.String("zone", "", "zone to rotate a key for")
	role := fs.String("role", "", `which key to rotate: "ksk" or "zsk" (omit to see the tradeoff explained first)`)
	keyPath := fs.String("key", "", "path to the current KSK")
	newKeyPath := fs.String("new-key", "", "(-role ksk) path to the new KSK (created if missing)")
	currentZSKPath := fs.String("current-zsk-key", "", "(-role zsk) path to the ZSK being replaced")
	newZSKPath := fs.String("new-zsk-key", "", "(-role zsk) path to the new ZSK (created if missing)")
	target := fs.String("target", "", "host:port, or an http(s):// URL for the §7.3 HTTPS carrier, to send the signed push to (omit to just self-verify)")
	jsonCarrier := addJSONCarrierFlag(fs)
	passphraseFile := addPassphraseFlag(fs)
	newPassphraseFile := fs.String("new-key-passphrase-file", "", "like -key-passphrase-file, but for the new key/ZSK")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *zone == "" {
		return fmt.Errorf("-zone is required")
	}

	if *role == "" {
		data := rotateKeyChoiceData{Zone: dns.Fqdn(*zone)}
		if *currentZSKPath != "" {
			if priv, err := sazu.LoadPrivateKey(*currentZSKPath, nil); err == nil {
				data.HasZSK = true
				data.ZSKKeyTag = sazu.DNSKEYFor(*zone, priv, false).KeyTag()
			}
		}
		printGuidance("rotate-key-choice.txt", data)
		return fmt.Errorf("no -role given -- see the explanation above, then re-run with -role ksk or -role zsk")
	}

	switch *role {
	case "zsk":
		if *keyPath == "" || *currentZSKPath == "" || *newZSKPath == "" {
			return fmt.Errorf("-role zsk needs -key (the KSK, or any authorized key), -current-zsk-key, and -new-zsk-key")
		}
		if err := runAddZSK([]string{
			"-zone", *zone, "-ksk-key", *keyPath, "-key-passphrase-file", *passphraseFile,
			"-zsk-key", *newZSKPath, "-zsk-key-passphrase-file", *newPassphraseFile,
			"-target", *target, jsonFlagArg(*jsonCarrier),
		}); err != nil {
			return fmt.Errorf("registering the new ZSK: %w", err)
		}
		fmt.Println()
		fmt.Println("New ZSK registered. Retiring the old one now:")
		fmt.Println()
		// Note: the old ZSK's own passphrase, if it has one, isn't
		// forwarded here -- rotate-key -role zsk only accepts one
		// passphrase flag pair (for the KSK and the new ZSK). Retire an
		// encrypted old ZSK directly with 'sazuctl retire-zsk
		// -zsk-key-passphrase-file' instead if that combination applies.
		if err := runRetireZSK([]string{
			"-zone", *zone, "-ksk-key", *keyPath, "-key-passphrase-file", *passphraseFile,
			"-zsk-key", *currentZSKPath,
			"-target", *target, jsonFlagArg(*jsonCarrier),
		}); err != nil {
			return fmt.Errorf("retiring the old ZSK (the new one is already registered and usable): %w", err)
		}
		return nil
	case "ksk":
		if *keyPath == "" || *newKeyPath == "" {
			return fmt.Errorf("-role ksk needs -key (the current KSK) and -new-key")
		}
		fmt.Println("KSK rotation requires a new DS record at your registrar, exactly like first onboarding this")
		fmt.Println("zone did -- if the push below is refused with a DS-related diagnostic, follow the guidance")
		fmt.Println("it prints (the new key's DS record, and how to publish it) before trying again.")
		fmt.Println()
		newPassphrase, err := readPassphraseFile(*newPassphraseFile)
		if err != nil {
			return err
		}
		newKSK, newPriv, generated, err := sazu.LoadOrGenerateKey(*newKeyPath, *zone, true, newPassphrase)
		if err != nil {
			return err
		}
		if generated {
			fmt.Fprintf(os.Stderr, "No key found at %s -- generated a new one.\n", *newKeyPath)
		}
		printKeyInfo(*newKeyPath, newKSK)

		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(*zone), dns.TypeSOA)
		m.Opcode = dns.OpcodeUpdate
		m.Insert([]dns.RR{&dns.DNSKEY{
			Hdr:   dns.RR_Header{Name: dns.Fqdn(*zone), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
			Flags: newKSK.Flags, Protocol: newKSK.Protocol, Algorithm: newKSK.Algorithm, PublicKey: newKSK.PublicKey,
		}})
		now := time.Now()
		wire, err := sazu.SignUpdate(m, newKSK, newPriv, now.Add(-time.Minute), now.Add(time.Hour))
		if err != nil {
			return err
		}
		return signSelfVerifyAndSend(*zone, wire, newKSK, *target, *jsonCarrier)
	default:
		return fmt.Errorf(`-role must be "ksk" or "zsk", got %q`, *role)
	}
}

// jsonFlagArg renders asJSON as the "-json" flag argument pair
// runAddZSK/runRetireZSK's own flag.FlagSet expects, or "" (a harmless
// no-op arg flag.Parse skips) when false -- a small helper so
// runRotateKey can forward its own -json choice to them without
// duplicating their flag-parsing logic.
func jsonFlagArg(asJSON bool) string {
	if asJSON {
		return "-json"
	}
	return "-json=false"
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
// its own key before sending anything, then sends it to target -- over
// UDP/TCP for a "host:port" target, or via §7.3's HTTPS/JSON carrier for
// an "http://"/"https://" URL target (asJSON selects the JSON wire
// envelope over that carrier instead of raw wire bytes) -- and reports
// what the server did with it, or just prints the wire bytes if no
// target was given.
func signSelfVerifyAndSend(zone string, wire []byte, key *dns.DNSKEY, target string, asJSON bool) error {
	if err := sazu.VerifySIG0(wire, key); err != nil {
		return fmt.Errorf("self-verification failed (this would be a bug): %w", err)
	}
	fmt.Printf("Self-verification: OK (%d bytes)\n", len(wire))

	if target == "" {
		fmt.Printf("No -target given; wire bytes (hex):\n%x\n", wire)
		return nil
	}

	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return sendOverHTTP(zone, wire, key, target, asJSON)
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

// sendOverHTTP sends wire to target (an "http://" or "https://" URL,
// with §7.3's DoH-style path appended) via a POST -- either raw
// application/dns-message bytes (the RFC 8484 DoH convention, reused
// as-is; the default) or, with asJSON, a doh.JSONWireEnvelope
// ({"wire": "<base64>"}). Both carry the identical, byte-exact wire
// bytes SIG(0) was computed over -- see plugin/pkg/doh's own doc
// comments for why this is deliberately never a structural (RFC 8427)
// JSON translation of the message's fields.
func sendOverHTTP(zone string, wire []byte, key *dns.DNSKEY, target string, asJSON bool) error {
	url := strings.TrimRight(target, "/") + doh.Path

	var body io.Reader
	contentType := doh.MimeType
	carrier := "raw wire bytes"
	if asJSON {
		envelope, err := json.Marshal(doh.JSONWireEnvelope{Wire: base64.StdEncoding.EncodeToString(wire)})
		if err != nil {
			return fmt.Errorf("marshaling JSON wire envelope: %w", err)
		}
		body = bytes.NewReader(envelope)
		contentType = doh.JSONMimeType
		carrier = "a JSON wire envelope"
	} else {
		body = bytes.NewReader(wire)
	}

	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("No response (%v) -- fine if nothing is listening yet; "+
			"the push itself encoded, signed, and self-verified correctly.\n", err)
		return nil
	}
	defer resp.Body.Close()
	fmt.Printf("Sent %d bytes to %s as %s (HTTP status %d)\n", len(wire), url, carrier, resp.StatusCode)

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}
	respMsg := new(dns.Msg)
	if err := respMsg.Unpack(buf); err != nil {
		fmt.Printf("Response (%d bytes, did not parse as a DNS message: %v):\n%x\n", len(buf), err, buf)
		return nil
	}
	return interpretResponse(zone, key, respMsg)
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
	printGuidance("no-ds.txt", dsGuidanceDataFor(zone, key))
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
	printGuidance("unknown-signer.txt", dsGuidanceDataFor(zone, key))
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
