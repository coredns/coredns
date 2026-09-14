# sazu

## Name

*sazu* - implements SAZU (Self-Authenticated Zone Update): a customer's own
signer pushes DNSSEC-signed zone content to this server, authenticated purely
by SIG(0) (RFC 2931) riding on an RFC 2136 dynamic UPDATE, with no separate
account or API-key handshake. The server never holds a private key.

See the design document (`sazu-protocol.md` in
[github.com/mrwiora/sazu](https://github.com/mrwiora/sazu), the separate
repo this port was built against) for the full protocol; see this repo's
own `SAZU-PLAN.md` for exactly what of it this port implements today,
including chain-of-trust bootstrap, full and partial pushes, key
rollover, rate limiting, persistence, an audit trail, the §11
delegation-change watch daemon, and pushing over UDP, TCP, or HTTPS
(§7.3, raw wire bytes or a JSON envelope). Treat this as a working proof
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
    rate_limit FULL_PER_DAY DIFFERENTIAL_PER_DAY
}
```

* **ZONES** the *scope* this instance accepts SAZU pushes and queries
  for — not a fixed list of pre-declared domains. Use `.` to accept
  onboarding any domain at all, with no Corefile edit or server restart
  needed per new customer domain: which specific zones actually exist is
  entirely driven by what's been onboarded at runtime (in `Store`, and in
  the `db` file if configured), not by this list. Use a narrower zone
  (e.g. `customers.example.`) to restrict onboarding to subdomains
  delegated under one umbrella zone instead. If empty, the zones from the
  server block are used.
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
* `rate_limit FULL_PER_DAY DIFFERENTIAL_PER_DAY` overrides §12's per-zone
  push quotas, each enforced over a rolling 24h window: FULL_PER_DAY for a
  full-zone push (`push-zone`, or first contact) and DIFFERENTIAL_PER_DAY
  for an ordinary partial one (`push-update`), tracked independently.
  Defaults to `5 50` if omitted. An exceeded quota is refused with the
  `ERR_QUOTA_EXCEEDED` diagnostic. Not persisted across a restart.

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
* `sazuctl push-zone -zone <zone> -key <path> -zonefile <path> [-previous-serial N] [-target host:port|url] [-json]` —
  build, sign, and (optionally) send a **full-zone** push: every record in a
  BIND-format zone file, plus the signing key as a DNSKEY. This is what
  onboards a zone (first contact) and what re-publishes a whole zone
  afterward. `-previous-serial` adds the SOA-serial staleness guard for a
  *re*-push against an already-onboarded zone; omit it for first contact.
  `-zonefile` is required — author a small BIND-format zone file (SOA plus
  whatever records you're onboarding) even for a brand-new domain; there is
  no synthesized-SOA shortcut.
* `sazuctl push-update -zone <zone> -key <path> [-add "rr"]... [-del "rr"]... [-del-rrset "name TYPE"]... [-target host:port|url] [-json]` —
  build, sign, and (optionally) send a **partial** push: individual
  add/delete operations against an already-onboarded zone. No DNSKEY is
  included — the server verifies against the key it already pinned.
* `sazuctl push -zone <zone> -key <path> [-record name=ipv4] [-target host:port|url] [-json]` —
  the original minimal single-record demo, kept for quick protocol
  smoke-testing. It does **not** include a SOA, so it cannot by itself
  onboard a zone against this server (see `push-zone` for that).
* `sazuctl contact -zone <zone> -key <path> [-address mailto:you@example.org]... [-clear] [-target host:port|url] [-json]` —
  register (or, with `-clear`, remove) the zone's §10.6 contact address(es):
  where `sazu-watchd`'s (§11) delegation-change alerts get sent.
  `-address` accepts `mailto:` for email or `http(s)://` for a webhook, and
  can repeat. This rides an ordinary authenticated push at a reserved owner
  name (`_sazu-contact.<zone>`) — it is never itself DNSSEC-signed or
  servable DNS content, just metadata carried alongside a real update.

Every subcommand without `-target` just prints the signed wire bytes and
self-verifies — safe to run with nothing listening yet.

`-target` accepts either `host:port` (sent over UDP, or TCP automatically
for anything too large for a single UDP datagram) or an `http://`/`https://`
URL — §7.3's HTTPS carrier, POSTed to `<url>/dns-query` exactly like a DoH
client would, reusing the RFC 8484 convention as-is: no separate account or
authorization step, the same SIG(0)-signed push either way. `-json` sends a
small JSON envelope (`{"wire": "<base64>"}`) instead of a raw
`application/dns-message` body when pushing over HTTPS — the exact same
wire bytes either way, never a structural re-encoding of the message (see
`plugin/pkg/doh`'s own doc comments for why: SIG(0) signs literal wire
bytes, and a structural JSON translation has no lossless way back to
them). A Corefile only needs an `https://` (or `https3://`) server block
with a `sazu` directive in it to accept these — see **Syntax** above,
nothing carrier-specific to configure.

Every subcommand that reads or writes a key file (`keygen`'s `-out`, every
other subcommand's `-key`) also accepts `-key-passphrase-file <path>`
(§10.8): give it and that key file is encrypted at rest (scrypt + AES-256-
GCM) with the passphrase in the given file, instead of the plain
BIND-format file `sazuctl` writes by default. Omit it and nothing changes
from before this existed.

`plugin/sazu/cmd/sazu_stub_tld` is a minimal stand-in parent zone, useful for
manually checking DS-digest wire correctness offline. It **cannot** be used
to satisfy chain-of-trust validation itself, which always walks the real DNS
root — see the next two sections for how to actually test that.

### sazu-watchd: §11 delegation-change monitoring

`plugin/sazu/cmd/sazu_watchd` is a separate, standalone daemon -- never runs
inside CoreDNS -- that periodically re-checks every onboarded zone's chain
of trust (the same "does a DS matching this zone's pinned key exist at the
parent" check first contact and a key rollover already perform) and alerts
the zone's registered contact (`sazuctl contact`) when that check's outcome
changes: a pinned key's DS silently disappearing or changing at the
registrar, without anyone re-pushing anything to this server, is exactly
the kind of drift nothing else here would ever notice.

```
go build -o sazu-watchd ./plugin/sazu/cmd/sazu_watchd
./sazu-watchd -db /path/to/the/same/sazu.db/CoreDNS/uses \
    -interval 5m \
    -smtp-addr smtp.example.org:587 -smtp-from alerts@example.org \
    -smtp-username alerts -smtp-password-file smtp-pass.txt
```

It reads the exact same SQLite file CoreDNS's `sazu` plugin writes to via
its `db` directive (both processes can safely have it open at once) --
there is no other configuration to keep in sync between the two. Alerts go
to whatever address(es) a zone registered with `sazuctl contact`: `mailto:`
addresses via SMTP (configure `-smtp-*` above, or leave them unset --
email alerts are simply skipped, with a logged error, until they're
configured), `https://`/`http://` addresses via a small JSON webhook POST.
A zone with no registered contact still gets every check logged, just
with nothing to notify externally.

Pass `-once` to run a single check pass and exit, instead of looping
forever -- useful for confirming the daemon can actually reach and parse
the database before wiring it into a real supervisor/systemd unit. Note
that `-once` resets its "last known good" state every time it starts (see
`watch.go`), so it never fires an alert on its own by design -- it exists
for connectivity/config verification, not as a substitute for the
long-running `-interval` loop a real deployment wants.

### Local sandbox testing

This is the fastest way to prove the whole mechanism works, using
`insecure_skip_chain_validation` since there's no real parent zone in a
sandbox to publish a DS record against.

1. **Start the server.** `sazu .` accepts onboarding *any* domain --
   nothing about which domain(s) you'll actually test needs deciding, or
   editing into the Corefile, up front:

   ```
   cat > Corefile <<'EOF'
   .:15353 {
       bind 127.0.0.1
       sazu . {
           insecure_skip_chain_validation
       }
       log
       errors
   }
   EOF
   ./coredns -conf Corefile
   ```

   (The steps below onboard `example.org` as a concrete example, but that
   name is chosen when you run `sazuctl`, not when you started the server
   above -- any other domain would work against this same, unmodified
   Corefile and running server.)

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

1. **Start the server** — same as the sandbox walkthrough, but with
   `insecure_skip_chain_validation` **removed** (this is the whole point of
   testing in the real world). `sazu .` still means no domain name needs
   deciding or editing into the Corefile up front — including onboarding
   more than one real domain later, with no second server block or restart:

   ```
   cat > Corefile <<'EOF'
   .:15353 {
       bind 127.0.0.1
       sazu .
       log
       errors
   }
   EOF
   ./coredns -conf Corefile
   ```

   (A narrower scope, e.g. `sazu yourdomain.example`, works the same way if
   you'd rather this instance only ever accept that one domain — see
   **Syntax** above.)

   The server needs outbound UDP/53 reachability to the internet (real root
   and TLD servers) for the chain walk to succeed — the usual case for any
   machine with normal internet access, but worth checking explicitly if
   this runs somewhere with restrictive egress rules.

   It also needs **inbound TCP/53 reachable**, not just UDP/53: `sazuctl`
   sends anything over roughly 1.2 KB over TCP automatically (see
   `push.go`/`cmd/sazuctl`), since a real signed push routinely exceeds
   the path MTU and gets silently dropped as an IP fragment on UDP —
   found the hard way against a real security-group-restricted host. If
   `push-zone` reports no response at all (not even a denial) against a
   server you otherwise know is up, check that inbound TCP/53 specifically
   isn't blocked, separately from UDP/53.

2. **If this domain is currently live with real traffic on it, read
   [Migrating an already-live domain](REGISTRARS.md#migrating-an-already-live-domain)
   in `REGISTRARS.md` before doing anything else in this section.**
   Publishing a DS record for a domain whose current host isn't also
   serving matching signatures breaks the *entire* domain — not just
   DNSSEC lookups — for every validating resolver, until that's fixed. A
   brand-new domain with no live traffic yet has none of this risk and can
   skip straight to the next step.

3. **Just try onboarding it.** You don't need to generate a key or fetch a
   DS record up front — `push-zone` does that for you and, on a domain
   with no DS published yet, tells you exactly what to do next (including
   the live-migration warning from the previous step, inline, if you skip
   reading it up front):

   ```
   ./sazuctl push-zone -zone yourdomain.example -key client.private \
       -zonefile yourdomain.example.zone -target 127.0.0.1:15353
   ```

   The first attempt against a real, not-yet-onboarded domain is *expected*
   to be denied — that's the chain-of-trust cross-check working correctly,
   not a bug. `sazuctl` generates the key (if `client.private` doesn't
   exist yet), prints the exact DS record to give your registrar (plus the
   key's raw fields — type, algorithm, key tag, public key — for a
   registrar like AWS Route 53 that asks you to enter those by hand
   instead of pasting a DS record), and points you at `REGISTRARS.md` for
   registrar-specific steps.

   A different denial is also possible here: if a DS record already exists
   for this domain but doesn't match this key (`ERR_UNKNOWN_SIGNER`),
   `sazuctl` prints separate guidance for that instead — it usually means
   DNSSEC is already enabled for this domain under a different key
   (possibly its current host's own, if you followed step 2 above), not
   necessarily anything wrong with this key. Onboarding isn't blocked by
   this: `sazuctl` tells you to add this key's DS record *alongside* the
   existing one (most registrars accept more than one) rather than
   replacing it, so you can proceed the same way as step 3 above without
   disturbing whatever's already keeping the domain validated.

4. **Submit that DS record at your registrar** — every major registrar
   that supports DNSSEC has a form for this (look for "DS record,"
   "DNSSEC," or "delegation signer"); see `REGISTRARS.md` for what's
   documented so far per registrar. **This is the one genuinely manual,
   out-of-band step** — ordinary DNSSEC hygiene, not something SAZU
   replaces.

5. **Wait for it to propagate**, then confirm with `dig DS yourdomain.example
   +short` as the message above says, and **re-run the exact same
   `push-zone` command from step 3.** Once the DS is visible, the same
   command that was denied now succeeds:

   ```
   Self-verification: OK (420 bytes)
   Sent 420 bytes to 127.0.0.1:15353
   Accepted (NOERROR).
   ```

   Your zone is now onboarded — verify with `dig @127.0.0.1 -p 15353 ...`
   exactly as in the sandbox walkthrough.

6. **Onboard your real zone content** and confirm the response is NOERROR,
   not REFUSED:

   ```
   ./sazuctl push-zone -zone yourdomain.example -key client.private \
       -zonefile yourdomain.example.zone -target 127.0.0.1:15353
   ```

   A REFUSED response here most likely means the DS isn't visible yet
   (recheck step 4), or the digest doesn't match the key you generated
   (recheck step 3).

7. **Verify and iterate** with `dig @127.0.0.1 -p 15353 ...` and
   `sazuctl push-update` exactly as in the sandbox walkthrough.

At no point in this flow does your domain's real, currently-serving
delegation change — this test server is never in the actual query path for
anyone but you, deliberately, so a mistake here can't take your domain
offline.

### Key rollover

A zone's key isn't permanent once pinned: generate a new one, publish
its DS record at your registrar alongside the existing one (most accept
more than one, and both stay listed throughout — see
`REGISTRARS.md`), wait for it to propagate, then push signed with the
new key, introducing it the same way first contact does:

```
./sazuctl keygen -out new-client.private -zone yourdomain.example
./sazuctl push-update -zone yourdomain.example -key new-client.private \
    -add "yourdomain.example. 3600 IN DNSKEY ..." -target 127.0.0.1:15353
```

(`sazuctl push-zone -key new-client.private ...` also works, and is
simpler if you're re-pushing full zone content at the same time — a
DNSKEY at the apex is exactly what BuildFullZonePush already always
includes.) The server verifies the new key both signs this push and has
a matching DS at the parent — the identical check first contact itself
requires — before switching over; until that succeeds, the old key keeps
working normally. Once switched, remove the old DS at your registrar
whenever you're ready; there's no rush, since a dangling extra DS
alongside the real one is safe (see `REGISTRARS.md`).

## Known limitations

Worth being explicit about what this proof of concept does *not* cover, so
a real-world test isn't mistaken for a production trial run:

* **A single mutex serializes every UPDATE** this plugin instance handles,
  across all zones. Fine for testing; a production version would want
  per-zone locking for throughput.
* **In-memory only if `db` is omitted.** Persistence via `db PATH` (SQLite)
  is available and tested; without it, restarting the server loses every
  onboarded zone and pinned key.
* **NSEC, not NSEC3** for authenticated denial of existence. NSEC3 exists
  to additionally hide a zone's name set from enumeration ("zone
  walking"); that's a real but separate, opt-in privacy property, not
  something a correct NXDOMAIN/NODATA proof requires, so it isn't
  implemented.
* **A partial push (`push-update`) invalidates the zone's NSEC chain
  until the next full push.** Only a full push (`push-zone`) ever
  computes one, since only it sees the entire zone's name set at once;
  see SAZU-PLAN.md for why a partial push can't safely patch the existing
  chain instead of just discarding it. Negative answers still work
  correctly in between, they just carry no DNSSEC denial-of-existence
  proof until the next full push.
