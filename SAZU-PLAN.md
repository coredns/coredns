# SAZU on CoreDNS — done and outstanding

SAZU (Self-Authenticated Zone Update): a customer's own signer pushes
DNSSEC-signed zone content to this server, authenticated purely by SIG(0)
(RFC 2931) riding on RFC 2136 dynamic UPDATE, with no separate account/API-key
handshake. Full protocol design lives in the separate
[github.com/mrwiora/sazu](https://github.com/mrwiora/sazu) repo
(`sazu-protocol.md`); this document tracks the Go/CoreDNS implementation
specifically (`plugin/sazu/`), branch `feat/sazu-test`.

## Done

All of the following is real, tested code — see `plugin/sazu/*_test.go` (29+
tests, including 4 full end-to-end tests that start a real `dnsserver.Server`
and drive it over actual UDP) and `plugin/sazu/README.md` for a manually
verified real-binary walkthrough.

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
- **Guided onboarding UX + `ERR_NO_DS_PUBLISHED` status code** (a first,
  minimal slice of §12's audit-trail/status-code item, not the whole
  thing). `chain.go` now distinguishes "the target zone's parent
  authoritatively publishes no DS at all" (`errNoDSRecords`, re-tagged as
  `ChainError{Op: "no-ds-published"}` only for the *final* DS check, so an
  unrelated break higher up the chain isn't confused with it) from every
  other way the chain-of-trust check can fail. `handler.go` carries that
  as a `TXT` diagnostic (`ERR_NO_DS_PUBLISHED`, exactly the design doc's
  own status-code name) in the response's Additional section alongside
  `REFUSED`. `sazuctl` reads it and prints the DS record plus concrete
  next steps instead of a bare failure, pointing at the new
  `plugin/sazu/REGISTRARS.md` (a placeholder today — no registrar-specific
  walkthroughs written yet, but a real place for them to live, referenced
  by name so the CLI message isn't pointing at nothing). `Sazu.Validator`
  is now the small `ChainValidator` interface rather than `*Validator`
  directly, so this response-shaping logic has its own tests using a fake
  validator, with no real network needed. Manually verified against the
  real chain-of-trust walk with a real, DNSSEC-less domain (`rust-lang.org`,
  not controlled by this project): a genuine first-contact push against it
  is denied with the exact guidance above; a domain with a real DS but the
  wrong key classifies as a different, undiagnosed rejection, proving the
  two cases don't get confused.
- **Onboarding a new domain needs no local file, client- or server-side.**
  Client-side: `sazuctl push-zone`'s `-zonefile` is now optional --
  `SynthesizeSOA` (`push.go`) builds a reasonable default SOA (and the
  caller adds a matching NS record) when there's nothing pre-authored on
  disk, driven entirely by `-zone`/`-ns`/repeatable `-add` flags instead.
  Server-side: fixed a real bug where zone routing conflated "which
  static Corefile entry matched" with "which zone a request is actually
  about" -- under a wildcard `sazu .` scope (meant to accept onboarding
  *any* domain with no Corefile edit per customer), every distinct
  domain used to collapse onto the single literal zone "." itself.
  `Store.FindZoneForName` now does the zone lookup against what's
  actually been onboarded at runtime, independent of the plugin's static
  configured scope; a name within that scope but never onboarded falls
  through to the next plugin rather than a false authoritative NXDOMAIN,
  so a broad `.` scope can't swallow every other zone on the same
  server. Manually verified with the real binary: one `sazu .` Corefile
  entry, two different domains onboarded back to back with no
  zone files and no Corefile changes between them, both served
  correctly and independently; a third, never-onboarded domain falls
  through rather than getting a false NXDOMAIN from this plugin.

## Outstanding

Split by where each belongs, per the architectural review that led to this
document. The one item marked **separate server** is the exception; every
other outstanding item is a CoreDNS-plugin change.

### CoreDNS-side

- [ ] **Registration record: key + contact address together (§10.6).** No
  contact field exists anywhere yet. Needed both for its own sake and
  because the separate watch daemon (below) needs to read it. Depends on
  persistence landing first.
- [ ] **DNSSEC-aware query serving** — RRSIG attachment on answers, EDNS
  DO-bit awareness. More fundamental than a "verification level": without
  this, a zone isn't actually usable as a signed zone by a validating
  resolver even if the customer pushed RRSIGs, because `serveQuery` doesn't
  attach them or even look at the DO bit today.
- [ ] **Content verification, Levels 1/2** (§4's orthogonal axis). Currently
  Level 0 ("trust the pipe"): SIG(0) proves who sent the UPDATE, but nothing
  checks the pushed RRs carry valid RRSIGs at all. The design doc's own
  recommendation is Level 1 first, Level 2 before trusting this for a zone
  anyone else depends on.
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
  (§12).** `ERR_NO_DS_PUBLISHED` is done (see Done, above) — the rest of
  the list isn't: no `ERR_STALE_SERIAL`, `ERR_UNKNOWN_SIGNER`,
  `ERR_SIG_INVALID`, `ERR_EXPIRED_SIGNATURE`, `ERR_WEAK_ALGORITHM`,
  `ERR_QUOTA_EXCEEDED`, or `ERR_RATE_LIMITED` yet (most of these are
  blocked on the features that would produce them, e.g. rate limiting
  below), and no per-transaction UUID or persistent audit log of
  accepted/rejected transactions.
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
