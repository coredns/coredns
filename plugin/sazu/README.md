# sazu

## Name

*sazu* - implements SAZU (Self-Authenticated Zone Update): a customer's own
signer pushes DNSSEC-signed zone content to this server, authenticated purely
by SIG(0) (RFC 2931) riding on an RFC 2136 dynamic UPDATE, with no separate
account or API-key handshake. The server never holds a private key.

See the design document (`sazu-protocol.md`, in the separate `sazu` design
repo this port was built against) for the full protocol. This plugin
implements enough of it — first-contact chain-of-trust bootstrap, full and
partial pushes, in-memory serving — to exercise the whole chain end to end.
It does **not** implement the operational surface the design treats as
separate concerns: rate limiting/quotas (§12), the §11 watch loop, key
rollover (§10.4), or the HTTPS carrier (§7.3). Treat this as a working proof
of concept for testing the mechanism, not a production-ready deployment.

## Description

*sazu* accepts RFC 2136 dynamic UPDATE messages for the zones it's configured
for. The **first** UPDATE for a zone establishes trust: it must carry a
DNSKEY record at the zone apex and a SOA record, be signed with SIG(0) using
that same key, and — unless chain validation is disabled for local testing —
the key must match a DS record published for that zone by its real parent
zone (walked all the way from the DNS root). Once that succeeds, the key is
*pinned*: every later UPDATE for that zone must be signed by the same key,
checked by cryptographic signature alone, with no re-check against the
parent chain on each push.

*sazu* also answers ordinary queries for the zones it has onboarded, directly
from the content it has accepted.

## Syntax

```
sazu ZONES... {
    insecure_skip_chain_validation
    db PATH
}
```

* **ZONES** zones this plugin accepts SAZU pushes for and serves. If empty,
  the zones from the server block are used.
* `insecure_skip_chain_validation` disables the §10.2 chain-of-trust
  cross-check at first contact. **For local testing only** — see
  [Local sandbox testing](#local-sandbox-testing) below. Never set this in
  production: with it set, *any* self-signed key claiming *any* zone name is
  accepted on first contact, which is exactly the spoofable behavior the
  cross-check exists to prevent.
* `db PATH` persists every onboarded zone and pinned key to a SQLite
  database at PATH (created if it doesn't exist), so a restart doesn't
  forget them. **Omit this and everything is purely in-memory** — lost on
  every restart, which is fine for a quick one-off test but not for
  anything you want to survive a redeploy.

## Examples

Building the server and client, then onboarding a zone locally, verifying
it, sending a partial update, and finally testing chain-of-trust validation
against a real domain.

### Building the server

From the root of this checkout:

```
go generate coredns.go   # only needed if plugin.cfg has changed
go build -o coredns .
```

The `sazu` directive is already registered in `plugin.cfg`; a normal
`go build .` at the repo root produces a `coredns` binary with it included.

### The client: sazuctl

`plugin/sazu/cmd/sazuctl` is the customer-side tool — it never runs inside
CoreDNS, and in a real deployment runs on the customer's own infrastructure,
never the hoster's.

```
go build -o sazuctl ./plugin/sazu/cmd/sazuctl
```

Subcommands:

* `sazuctl keygen -out <path> [-zone <owner>]` — generate a new Ed25519
  SIG(0)/zone key, saved in BIND9's private-key-file format.
* `sazuctl ds -zone <zone> -key <path>` — print the DS record for a key,
  ready to hand to a registrar. Generates the key first if it doesn't exist.
* `sazuctl push-zone -zone <zone> -key <path> -zonefile <path> [-previous-serial N] [-target host:port]` —
  build, sign, and (optionally) send a **full-zone** push: every record in a
  BIND-format zone file, plus the signing key as a DNSKEY. This is what
  onboards a zone (first contact) and what re-publishes a whole zone
  afterward. `-previous-serial` adds the SOA-serial staleness guard for a
  *re*-push against an already-onboarded zone; omit it for first contact.
* `sazuctl push-update -zone <zone> -key <path> [-add "rr"]... [-del "rr"]... [-del-rrset "name TYPE"]... [-target host:port]` —
  build, sign, and (optionally) send a **partial** push: individual
  add/delete operations against an already-onboarded zone. No DNSKEY is
  included — the server verifies against the key it already pinned.
* `sazuctl push -zone <zone> -key <path> [-record name=ipv4] [-target host:port]` —
  the original minimal single-record demo, kept for quick protocol
  smoke-testing. It does **not** include a SOA, so it cannot by itself
  onboard a zone against this server (see `push-zone` for that).

Every subcommand without `-target` just prints the signed wire bytes and
self-verifies — safe to run with nothing listening yet.

`plugin/sazu/cmd/sazu_stub_tld` is a minimal stand-in parent zone, useful for
manually checking DS-digest wire correctness offline. It **cannot** be used
to satisfy chain-of-trust validation itself, which always walks the real DNS
root — see the next two sections for how to actually test that.

### Local sandbox testing

This is the fastest way to prove the whole mechanism works, using
`insecure_skip_chain_validation` since there's no real parent zone in a
sandbox to publish a DS record against.

1. **Start the server.**

   ```
   cat > Corefile <<'EOF'
   .:15353 {
       bind 127.0.0.1
       sazu example.org {
           insecure_skip_chain_validation
       }
       log
       errors
   }
   EOF
   ./coredns -conf Corefile
   ```

2. **Generate a key and check it.**

   ```
   ./sazuctl keygen -out client.private -zone example.org
   ```

3. **Write a small zone file and onboard it** (first contact — a full push,
   no `-previous-serial`):

   ```
   cat > example.org.zone <<'EOF'
   $ORIGIN example.org.
   @   3600 IN SOA ns1.example.org. hostmaster.example.org. 2024010100 3600 900 604800 3600
   @   3600 IN NS  ns1.example.org.
   www 300  IN A   203.0.113.10
   EOF

   ./sazuctl push-zone -zone example.org -key client.private \
       -zonefile example.org.zone -target 127.0.0.1:15353
   ```

   A `Self-verification: OK` line followed by a 29-byte NOERROR response
   means the zone is onboarded and the key is pinned.

4. **Verify with dig** (or any DNS client — the server is a real,
   standards-compliant authoritative responder at this point):

   ```
   dig @127.0.0.1 -p 15353 www.example.org A
   dig @127.0.0.1 -p 15353 example.org SOA
   ```

5. **Send a partial update** and confirm it took effect:

   ```
   ./sazuctl push-update -zone example.org -key client.private \
       -add "mail.example.org. 300 IN A 203.0.113.20" \
       -target 127.0.0.1:15353

   dig @127.0.0.1 -p 15353 mail.example.org A
   ```

6. **Confirm impersonation is rejected**: generate a second, different key
   and try to push with it against the same zone — it must be refused
   (`NOTAUTH`), and the record must not appear:

   ```
   ./sazuctl push-update -zone example.org -key attacker.private \
       -add "evil.example.org. 300 IN A 198.51.100.1" \
       -target 127.0.0.1:15353

   dig @127.0.0.1 -p 15353 evil.example.org A   # should be NXDOMAIN
   ```

This exercises everything except the chain-of-trust walk itself (stubbed out
by `insecure_skip_chain_validation`). That part has its own dedicated,
network-based tests in `chain_test.go`/the package's other tests, and needs
a real domain to test live — see below.

### Testing in the real world

To test chain-of-trust validation for real, you need a domain with DNSSEC
enabled at your registrar. You do **not** need to change that domain's
actual nameserver delegation, and you do **not** need to run this server on
a public IP or port 53 — the chain-of-trust check only asks "does the
parent zone publish a DS record matching this key," which is a normal,
unauthenticated DNS question anyone can ask against the real DNS root; it
has nothing to do with who currently serves the domain's real traffic. So
you can point `sazuctl` at a test instance of this server running anywhere
reachable to you, on any port, while your domain keeps working normally
through its real nameservers throughout.

1. **Pick a domain you control that supports DNSSEC**, and generate a key:

   ```
   ./sazuctl keygen -out client.private -zone yourdomain.example
   ```

2. **Publish the DS record at your registrar.**

   ```
   ./sazuctl ds -zone yourdomain.example -key client.private
   ```

   This prints something like:

   ```
   yourdomain.example. IN DS 12345 15 2 <64-hex-char digest>
   ```

   Take the key tag, algorithm (15 = Ed25519), digest type (2 = SHA-256),
   and digest, and add them as a DS record through your registrar's control
   panel (every major registrar that supports DNSSEC has a form for this —
   look for "DS record," "DNSSEC," or "delegation signer"). **This step is
   the one genuinely manual, out-of-band part of onboarding** — it's
   ordinary DNSSEC hygiene, not something SAZU replaces.

3. **Wait for the DS to actually be visible**, since registrars propagate
   this at their own pace (minutes to a few hours is typical):

   ```
   dig DS yourdomain.example +short
   ```

   Don't proceed until this returns your digest — a chain-of-trust check
   before propagation completes will correctly fail with "no DS published
   yet," which is the right behavior, not a bug.

4. **Start the server** — same as the sandbox walkthrough, but with the
   Corefile pointed at your real domain and `insecure_skip_chain_validation`
   **removed** (this is the whole point of testing in the real world):

   ```
   cat > Corefile <<'EOF'
   .:15353 {
       bind 127.0.0.1
       sazu yourdomain.example
       log
       errors
   }
   EOF
   ./coredns -conf Corefile
   ```

   The server needs outbound UDP/53 reachability to the internet (real root
   and TLD servers) for the chain walk to succeed — the usual case for any
   machine with normal internet access, but worth checking explicitly if
   this runs somewhere with restrictive egress rules.

5. **Onboard your real zone content** and confirm the response is NOERROR,
   not REFUSED:

   ```
   ./sazuctl push-zone -zone yourdomain.example -key client.private \
       -zonefile yourdomain.example.zone -target 127.0.0.1:15353
   ```

   A REFUSED response here most likely means the DS isn't visible yet
   (recheck step 3), or the digest doesn't match the key you generated
   (recheck step 2).

6. **Verify and iterate** with `dig @127.0.0.1 -p 15353 ...` and
   `sazuctl push-update` exactly as in the sandbox walkthrough.

At no point in this flow does your domain's real, currently-serving
delegation change — this test server is never in the actual query path for
anyone but you, deliberately, so a mistake here can't take your domain
offline.

## Known limitations

Worth being explicit about what this proof of concept does *not* cover, so
a real-world test isn't mistaken for a production trial run:

* **No rate limiting or per-tenant quotas** (§12). Every configured zone
  shares one server instance with no throttling.
* **No key rollover** (§10.4). Once a key is pinned, there is no supported
  way to replace it short of restarting the server (which forgets all
  pinned keys and onboarded zones — this store is in-memory only, nothing
  persists across a restart).
* **No delegation-change watch loop** (§11). A pinned key that later drops
  out of the zone's real DNSKEY RRset at the parent isn't detected.
* **UDP only.** RFC 2136 updates are commonly sent over TCP for large
  zones; this plugin has not been tested against TCP.
* **A single mutex serializes every UPDATE** this plugin instance handles,
  across all zones. Fine for testing; a production version would want
  per-zone locking for throughput.
* **In-memory only if `db` is omitted.** Persistence via `db PATH` (SQLite)
  is available and tested; without it, restarting the server loses every
  onboarded zone and pinned key.
