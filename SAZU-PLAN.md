# SAZU on CoreDNS — done and outstanding

SAZU (Self-Authenticated Zone Update): a customer's own signer pushes
DNSSEC-signed zone content to this server, authenticated purely by SIG(0)
(RFC 2931) riding on RFC 2136 dynamic UPDATE, with no separate account/API-key
handshake. Full protocol design lives in the separate
[github.com/mrwiora/sazu](https://github.com/mrwiora/sazu) repo
(`sazu-protocol.md`); this document tracks the Go/CoreDNS implementation
specifically (`plugin/sazu/`), branch `feat/sazu-test`.

## Done

All of the following is real, tested code — see `plugin/sazu/*_test.go` (78+
tests, including several full end-to-end tests that start a real
`dnsserver.Server` and drive it over actual UDP) and `plugin/sazu/README.md`
for a manually verified real-binary walkthrough.

- **SIG(0) transaction authentication**, byte-exact, with no second listener.
  `sig0.go` wraps `miekg/dns`'s native `SIG.Sign`/`SIG.Verify`; `rawcapture.go`
  + the `UDPDecorateReaderFunc` hook added to `core/dnsserver` (its own,
  independently mergeable commit) solve "how does a plugin get the literal
  wire bytes a client sent," which RFC 2931 needs and a re-encoded `*dns.Msg`
  cannot guarantee reproduces. §7.2, §9.1.
- **First-contact chain-of-trust bootstrap.** `chain.go`/`trustanchor.go`
  walk from a hardcoded root trust anchor down to a zone's parent, verifying
  DNSKEY/DS RRsets and their RRSIGs at each level via real DNS queries with
  the DO bit set, and check a candidate key against the parent's DS records.
  §10.2.
- **Key pinning / anti-impersonation.** `keys.go`, `handler.go`. Once a key is
  pinned for a zone, only that key's signature is accepted; a push signed by
  a different key is rejected (`NOTAUTH`) and cannot touch the zone.
- **RFC 2136 wire mechanics**: all 5 prerequisite forms and all 4 update
  forms, decoded correctly from wire-accurate `Class`/`Rdlength` (`prereq.go`),
  with the right RFC 2136 §2.6 rcode per failure (NXRRSET/YXRRSET/NXDOMAIN/
  YXDOMAIN, not just success/failure).
- **Full-zone push**: `push.go`'s `LoadZoneFile`/`BuildFullZonePush` turn a
  real BIND zone file into a signed UPDATE (DNSKEY + SOA + every record),
  with an SOA-serial staleness guard (RFC 2136 §2.4.2) for re-pushes. §12.
- **Partial/differential push**: `sazuctl push-update` — add/delete
  individual records against an already-onboarded zone, no DNSKEY, verified
  against the already-pinned key. §12.
- **In-memory serving** of everything accepted (`store.go`), answering
  ordinary queries directly from what was pushed.
- **Client tooling** (`cmd/sazuctl`): `keygen`, `ds` (prints a registrar-ready
  DS record), `push`, `push-zone`, `push-update`.
- **Registered as a real CoreDNS plugin** (`plugin.cfg`, `setup.go`,
  regenerated `zdirectives.go`/`zplugin.go`) — a normal `go build .` produces
  a `coredns` binary with `sazu` in it, configurable from a Corefile.
- **Persistence** (`db.go`, the `db PATH` Corefile option): SQLite via
  `modernc.org/sqlite` (pure Go, no cgo). Every accepted UPDATE is
  transactional (`CommitUpdate`, all-or-nothing) and committed to disk
  *before* the in-memory `Store`/`KeyRegistry` are mutated, so a
  persistence failure can't leave memory and disk disagreeing;
  `LoadAll` replays everything back into fresh in-memory state at
  startup. `Store`/`KeyRegistry`/`prereq.go` themselves stay pure
  in-memory and untouched — this is a wrapper `handler.go`/`setup.go`
  add on top, not a rewrite. Manually verified end to end: onboarded a
  zone, killed and restarted the real `coredns` binary, confirmed the
  zone served correctly and the pinned key still rejected an
  impersonation attempt with no re-onboarding needed. Omitting `db`
  keeps the original pure in-memory behavior.
- **Guided onboarding UX + `ERR_NO_DS_PUBLISHED`/`ERR_UNKNOWN_SIGNER`
  status codes** (a first, still-partial slice of §12's audit-trail/
  status-code item, not the whole thing). `chain.go` distinguishes three
  outcomes of the final DS check: "the target zone's parent
  authoritatively publishes no DS at all" (`errNoDSRecords`, re-tagged as
  `ChainError{Op: "no-ds-published"}`), "a DS is published but doesn't
  match the candidate key" (`ChainError{Op: "key-mismatch"}`), and every
  other, generic chain-of-trust failure (broken ancestor, network error,
  etc.), which stays undiagnosed on purpose. `handler.go` carries the
  first two as `TXT` diagnostics (`ERR_NO_DS_PUBLISHED` /
  `ERR_UNKNOWN_SIGNER`, the design doc's own status-code names) in the
  response's Additional section alongside `REFUSED`. `sazuctl` reads
  either and prints dedicated next steps instead of a bare failure,
  pointing at `plugin/sazu/REGISTRARS.md`. `ERR_UNKNOWN_SIGNER`'s guidance
  deliberately does not assume an attack: a DS that doesn't match this key
  is just as likely to be the zone's *current* host already publishing its
  own, unrelated DNSSEC — see the next bullet for why that specific case
  matters. It also doesn't just tell the operator to investigate and wait:
  since `VerifyChainOfTrust` accepts a candidate key as soon as *any*
  published DS matches it, the guidance gives the same concrete DS record
  as the no-DS case, framed as "add this alongside the existing DS, most
  registrars accept more than one (RFC 6781 §4.1.4 key/algorithm
  rollover) — don't remove the other one until actual cutover." Manually
  verified against `cloudflare.com` (a real domain with its own,
  unrelated DNSSEC already enabled): onboarding is correctly denied with
  this exact guidance rather than a bare, unhelpful `REFUSED`.
  `Sazu.Validator` is the small `ChainValidator` interface rather
  than `*Validator` directly, so this response-shaping logic has its own
  tests using a fake validator, with no real network needed. Manually
  verified against the real chain-of-trust walk with a real, DNSSEC-less
  domain (`rust-lang.org`, not controlled by this project): a genuine
  first-contact push against it is denied with the `ERR_NO_DS_PUBLISHED`
  guidance above.
- **Live-migration hazard documented, and flagged in `sazuctl`'s own
  output.** A real finding from testing against a live domain
  (sinepress.org): if a domain currently has *no* DNSSEC at all and is
  still being served by its current (non-SAZU) host, publishing a DS
  record for the SAZU key breaks the **entire domain** — not just DNSSEC
  lookups — for every validating resolver, from the moment the DS
  propagates until the domain is actually, fully cut over to serving
  signed content from this server. There is no way for this server to
  detect or prevent that by itself (the breakage happens entirely outside
  it, at the domain's current, unrelated host), so this is handled by
  making sure the guidance is unmissable at exactly the two moments a
  client would otherwise walk into it blind: `sazuctl`'s
  `ERR_NO_DS_PUBLISHED` guidance (printed before a client is told to
  publish a DS at all) and its `ERR_UNKNOWN_SIGNER` guidance (printed if a
  pre-existing, unrelated DS is found instead), plus a dedicated
  "Migrating an already-live domain" section in `REGISTRARS.md` referenced
  from both. The recommended mitigation: enable DNSSEC on the domain's
  *current* host first if it supports that (keeps the domain validly
  signed under its own key throughout the migration, with the SAZU DS only
  swapped in at actual cutover), or move hosting to one that supports
  enabling DNSSEC (e.g. AWS Route 53) if it doesn't. A brand-new domain
  with no live traffic yet has none of this risk.
- **Onboarding a new domain needs no server-side Corefile edit.** Fixed a
  real bug where zone routing conflated "which static Corefile entry
  matched" with "which zone a request is actually about" -- under a
  wildcard `sazu .` scope (meant to accept onboarding *any* domain with
  no Corefile edit per customer), every distinct domain used to collapse
  onto the single literal zone "." itself. `Store.FindZoneForName` now
  does the zone lookup against what's actually been onboarded at
  runtime, independent of the plugin's static configured scope; a name
  within that scope but never onboarded falls through to the next plugin
  rather than a false authoritative NXDOMAIN, so a broad `.` scope can't
  swallow every other zone on the same server. Manually verified with
  the real binary: one `sazu .` Corefile entry, two different domains
  onboarded back to back with no Corefile changes between them, both
  served correctly and independently; a third, never-onboarded domain
  falls through rather than getting a false NXDOMAIN from this plugin.
  (Client-side, onboarding still requires a real zone file --
  `sazuctl push-zone -zonefile <path>` -- an earlier synthesized-SOA
  shortcut that skipped it was tried and then deliberately removed as
  more confusing than helpful; see git history if it's ever wanted
  back.)
- **Real DNSSEC content-signing, and DNSSEC-aware query serving.**
  `sign.go`'s `SignZoneContent` gives every RRset a full-zone push carries
  (DNSKEY, SOA, and content alike) a genuine RFC 4034 RRSIG, in both
  `BuildFullZonePush` and `sazuctl push-update`'s added records --
  previously SIG(0) authenticated the transaction but nothing signed the
  content itself. `serveQuery` now attaches the covering RRSIG(s) to an
  answer when the query's EDNS0 DO bit is set (`store.go`'s
  `LookupRRSIG`), and omits them otherwise -- closing the exact gap found
  diagnosing why sinepress.org was SERVFAIL (DS published, but nothing
  signed being served). This is §4's Level 1 (content is genuinely
  signed) plus DO-bit-aware serving; Level 2 (below) makes acceptance
  itself conditional on it.
- **Content verification, Level 2 (§4), opt-in.** `sign.go`'s
  `VerifySignedRRsets` plus a new `RequireValidRRSIGs` field on `Sazu`
  (Corefile: `require_valid_rrsigs`, zero-arg boolean, off by default) --
  when enabled, a push is rejected (`NOTAUTH` + the new `ERR_SIG_INVALID`
  diagnostic) unless every added RRset carries a covering RRSIG that
  actually verifies against the candidate/pinned key. Left off, "Level 0,
  trust the pipe" (SIG(0) alone) remains a supported, simpler mode. Also
  fixed a real, previously-undiscovered CoreDNS-wide bug this surfaced:
  `core/dnsserver` never raised `dns.Server`'s UDP receive buffer past
  miekg/dns's 512-byte default, silently truncating any signed push over
  that size; added `Config.UDPSize` (own commit, cleanly separable from
  the sazu-specific work) to fix it.
- **Fixed unbounded duplicate/RRSIG accumulation in `store.go`.** Found
  live, verifying a real onboarded zone (sinepress.org) against the DNSSEC
  standard end to end: `ZoneData.insertLocked` appended every inserted RR
  unconditionally, with no check for content already present -- a direct
  violation of RFC 2136 §3.4.2.2 ("In case of duplicate RDATAs ... the
  Zone RR is replaced by [the] Update RR"), confirmed on the wire as a
  literal duplicate A/DNSKEY/NS record served twice after two pushes of
  the same content. Cryptographic validation was unaffected only by luck:
  miekg/dns's own `RRSIG.Verify` already deduplicates identical wire-form
  records before hashing (RFC 4034 §6.2 canonical form), so the served
  signatures still verified -- but the underlying store bug was real, and
  had a second, worse consequence: since a *fresh* RRSIG always has
  different RDATA (a new signature and validity window) even over
  unchanged content, it was never caught by the RFC's literal
  duplicate-RDATA rule either, so every routine re-sign of a long-lived
  zone (expected periodically, given `DefaultSignatureValidity`'s 30-day
  window) would have accumulated one more RRSIG forever, with nothing
  ever pruning the old ones. Fixed by making `Insert` replace
  content-identical ordinary RRs in place (refreshing TTL, per the RFC),
  and by having a fresh RRSIG from a given signer replace that same
  signer's previous RRSIG over the same covered type at that name instead
  of accumulating beside it (scoped by signer/key/covered-type, so a
  second key's simultaneous signature, e.g. mid key rollover, still
  legitimately coexists). Verified against the real binary: three
  identical `push-zone` calls in a row now leave exactly one A record and
  one RRSIG being served, not three of each.

Split by where each belongs, per the architectural review that led to this
document. The one item marked **separate server** is the exception; every
other outstanding item is a CoreDNS-plugin change.

### CoreDNS-side

- [ ] **Registration record: key + contact address together (§10.6).** No
  contact field exists anywhere yet. Needed both for its own sake and
  because the separate watch daemon (below) needs to read it. Depends on
  persistence landing first.
- [ ] **Algorithm policy / weak-algorithm floor (§10.7).** No rejection of
  weak algorithms (e.g. SHA-1-only DS digests) anywhere in the Go port today
  — the Rust/rDNS port had `meets_minimum_floor()` checks (RFC 8624); it
  didn't carry over.
- [ ] **Key rollover (§10.4).** Once pinned, a key is permanent. Needs the
  same chain-of-trust-recheck machinery already built for first contact,
  triggered by a different condition (an already-pinned zone presenting a
  new candidate key that also chains to the parent's DS).
- [ ] **Rate limiting / quota (§12).** No throttling at all — 5 full-zone/day,
  50 differential/day per zone (customizable), 24h rolling window, per the
  design doc's starting numbers.
- [ ] **Audit trail, remaining transaction status codes, transaction UUID
  (§12).** `ERR_NO_DS_PUBLISHED`, `ERR_UNKNOWN_SIGNER`, and
  `ERR_SIG_INVALID` are done (see Done, above) — the rest of the list
  isn't: no `ERR_STALE_SERIAL`, `ERR_EXPIRED_SIGNATURE`,
  `ERR_WEAK_ALGORITHM`, `ERR_QUOTA_EXCEEDED`, or `ERR_RATE_LIMITED` yet
  (most of these are blocked on the features that would produce them,
  e.g. rate limiting below), and no per-transaction UUID or persistent
  audit log of accepted/rejected transactions.
- [ ] **HTTPS/JSON carrier, RFC 8427 (§7.3).** UDP wire format only today.
  Recommend plugging into CoreDNS's existing `https` plugin rather than a
  separate service — same authorization and zone state, just a different
  wire encoding.
- [ ] **TCP.** `AllowOpcode` already permits UPDATE over TCP at the
  `core/dnsserver` level, but `RawCapture` is UDP-only, so a TCP UPDATE
  reaches the handler today but always fails closed (no captured bytes to
  verify SIG(0) against) — safely, but uselessly. Needs `RawCapture`'s
  `DecorateReaderFunc` pattern extended to TCP reads.

### Client-side (not a server concern either way)

- [ ] **Key custody hardening (§10.8).** `sazuctl` writes a plain BIND-format
  key file today — no HSM support, no encryption at rest.

### Separate server

- [ ] **§11 Delegation-change monitoring & alerting — the watch loop /
  notification mechanism.** Not implemented at all. Design:

  A standalone daemon (`sazu-watchd`) that, per onboarded zone, every ~5
  minutes:
  1. Reads the zone's pinned key and registered contact address from shared
     storage (persistence above — the forcing function for building that
     first).
  2. Re-runs `chain.go`'s `Validator` (imported as a library, the same way
     `sazuctl` already imports `plugin/sazu`) to fetch the parent's current
     NS+DS.
  3. Compares against last-known-good; on divergence, logs and alerts the
     registered contact (email/webhook/etc — integration TBD).

  Kept out of CoreDNS deliberately: it's a periodic background job, not
  request-driven, and its failure mode (a slow/flaky query to some TLD
  server) must never be able to add latency to actual DNS answers or tie
  monitoring continuity to the query-serving process's uptime. It will read
  the persistence layer above (zones/keys) once the registration-record
  item adds a contact address to persist alongside them.
