# SAZU on CoreDNS — done and outstanding

SAZU (Self-Authenticated Zone Update): a customer's own signer pushes
DNSSEC-signed zone content to this server, authenticated purely by SIG(0)
(RFC 2931) riding on RFC 2136 dynamic UPDATE, with no separate account/API-key
handshake. Full protocol design lives in the separate
[github.com/mrwiora/sazu](https://github.com/mrwiora/sazu) repo
(`sazu-protocol.md`); this document tracks the Go/CoreDNS implementation
specifically (`plugin/sazu/`), branch `feat/sazu-test`.

## Done

All of the following is real, tested code — see `plugin/sazu/*_test.go` (100+
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
  surfaced a real, previously-undiscovered CoreDNS-wide bug: `core/dnsserver`
  never raised `dns.Server`'s UDP receive buffer past miekg/dns's 512-byte
  default, silently truncating any signed push over that size. Initially
  fixed with a `Config.UDPSize` field -- since removed, see the TCP entry
  below for why a bigger UDP buffer turned out to be the wrong fix.
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
- **Negative responses (NXDOMAIN/NODATA) now carry the zone's SOA in the
  authority section**, signed when DO is set. Found the same way as the
  bug above: comparing this server's answers directly against a real
  authoritative server (AWS Route 53) for the same zone side by side. AWS
  returned `SOA + RRSIG(SOA) + NSEC + RRSIG(NSEC)` in Authority for a
  NODATA answer; this server returned nothing in Authority at all --
  not even the SOA RFC 2308 §3 requires there for negative caching,
  independent of DNSSEC entirely. Fixed `serveQuery` to add the zone's
  SOA (plus its RRSIG when DO is set) to `m.Ns` for both the NXDOMAIN and
  NODATA branches. This alone doesn't make a negative answer validate as
  secure -- that needed the NSEC work below, done as a direct follow-on.
  Verified against the real binary with the same query shape as the
  side-by-side comparison that found this (`A` query at a zone's apex,
  where only SOA/NS exist).
- **Authenticated denial of existence, via NSEC** (not NSEC3 -- see
  below for why). Closes the gap the previous bullet's comparison against
  AWS Route 53 found and explicitly left open: negative answers now carry
  a real, cryptographically valid NSEC (+ its RRSIG, when DO is set),
  matching AWS's own `SOA + RRSIG(SOA) + NSEC + RRSIG(NSEC)` shape
  exactly, verified against the real binary with the identical query.
  Genuinely harder for SAZU than for a provider like AWS: AWS can
  synthesize a covering NSEC on the fly, at answer time, because it holds
  the zone's private key; SAZU's server never does, so that's not an
  option here. Instead:
  - `nsec.go`'s `BuildNSECChain`, called from `BuildFullZonePush`,
    computes a complete, correctly-ordered NSEC chain (RFC 4034 §6.1
    canonical name order) from a full push's own content and folds it
    into the same `SignZoneContent` call as everything else -- the
    customer's own signer produces it, the same way traditional offline
    zone-signing tools (`dnssec-signzone`) do, since only a full push
    ever sees the zone's entire name set at once.
  - `store.go`'s `ZoneData.NegativeProof` serves the right already-signed
    record(s) at query time: for NODATA, the NSEC stored at the queried
    name itself; for NXDOMAIN, the NSEC covering the queried name plus
    the one covering the wildcard slot at its closest encloser (RFC 4035
    §3.1.3) -- meaningful even though SAZU never synthesizes
    wildcard-matched answers itself, since it's proving no wildcard
    *elsewhere in the zone* could have matched either.
  - **A partial push (`push-update`) never computes or includes NSEC
    records** -- only a full push sees the whole name set, so only a full
    push can be trusted to produce a *complete* chain. Rather than risk
    serving a stale chain that contradicts what a partial push just
    changed (a real danger: a stale NSEC's type bitmap could wrongly
    claim a just-deleted record type still exists, which is worse than no
    proof at all -- an actively wrong one), `ZoneData.PurgeNSEC` -- called
    before applying *any* update, full or partial -- invalidates the
    entire existing chain up front. A full push's own fresh chain
    repopulates it in the same update, immediately after; a partial push
    leaves the zone with no negative-existence proof at all until the
    next full push. A deliberate, documented trade of completeness for
    correctness, verified end to end (`TestPartialPushInvalidatesNSECUntilNextFullPush`).
    `db.go`'s `CommitUpdate` mirrors the same purge in SQL, so this holds
    across a restart, not just in memory.
  - `store.go`'s `insertLocked` also now treats NSEC as a singleton per
    name (like the existing RRSIG-replace logic, generalized) --
    necessary because two different NSEC values at the same name (e.g.
    from two different full pushes) are a replacement, not a legitimate
    second value the existing RFC 2136 "identical RDATA replaces" rule
    would ever recognize as such.
  - **NSEC3 is a deliberate non-goal for now.** It exists to additionally
    hide a zone's name set from enumeration ("zone walking"), which is a
    real but separate, opt-in privacy property -- not something a correct
    NXDOMAIN/NODATA proof requires. Plain NSEC is what actually resolves
    validating resolvers treating this server's negative answers as
    Bogus, which was the real problem.
- **TCP support for pushes, replacing the earlier `Config.UDPSize`
  workaround.** Found live against a real server: a genuine signed push
  well under `UDPSize`'s 16 KiB ceiling (around 1.5-2 KB) got *no
  response at all*, confirmed via a controlled size sweep to fail
  starting exactly around the ~1472-byte path MTU -- not a receive-buffer
  truncation (which at least produces `FORMERR`), but the message getting
  fragmented at the IP layer and the fragments silently dropped
  somewhere in the network path (a very common security posture: many
  firewalls and security groups drop non-initial UDP fragments). No
  server-side receive-buffer size can fix a problem that happens before
  the packet ever arrives. The actual, standards-correct fix -- TCP has
  been DNS's designated fallback transport for oversized messages since
  RFC 1035 itself, formalized as a requirement in RFC 7766, and is
  exactly the direction the 2020 "DNS Flag Day" industry consensus (BIND,
  PowerDNS, Knot, Unbound) pushed the whole ecosystem for the same
  underlying reason on the response side:
  - `core/dnsserver.Config.TCPDecorateReaderFunc` (own commit, cleanly
    separable, on `feat/tcp-decorate-reader` off `master`): CoreDNS's TCP
    listener never wired `DecorateReader` through to the underlying
    `dns.Server` at all, unlike UDP, so there was no way to get
    byte-exact request bytes for SIG(0) verification over TCP even in
    principle. Added, mirroring `UDPDecorateReaderFunc` exactly.
  - `rawcapture.go`'s `RawCapture`/`capturingReader` needed almost no
    change: entries are keyed by address + message ID regardless of
    transport, so the one missing piece was `ReadTCP` actually calling
    `Put` (it was a silent passthrough before). The same `RawCapture`
    instance and the same `DecorateReaderFunc` now serve both
    `UDPDecorateReaderFunc` and `TCPDecorateReaderFunc`.
  - `sazuctl` now picks the transport automatically by size
    (`safeUDPPushSize`) rather than always using UDP: small pushes (most
    `push-update` calls) stay on UDP: fewer round trips, no connection
    overhead; anything larger (most `push-zone` full pushes, especially
    now that a real NSEC chain is included) goes over TCP automatically,
    with RFC 1035 §4.2.2's 2-byte length-prefix framing. There is no
    "split one UPDATE across several UDP datagrams" mechanism in RFC 2136
    or any real implementation -- escalating transport, not shrinking the
    message, is the only real option once a push is this size.
    `safeUDPPushSize` is **512 bytes, not the 1232-byte "DNS Flag Day"
    value it was first set to** -- a real bug found immediately after
    landing this: 1232 is the safe ceiling for *response* sizes once a
    receive buffer is raised to match it, which nothing on the request
    side does here (that's the whole point of removing `Config.UDPSize`
    below). A push between 512 and 1232 bytes still went out over UDP,
    still got silently truncated to exactly 512 bytes on receipt
    (miekg/dns's real, unmodified default), and failed with a low-level
    `FORMERR` indistinguishable from a genuinely malformed request --
    reproduced locally byte-for-byte against a real user's zone file and
    key. `safeUDPPushSize` has to track the server's actual receive
    capacity, not a value borrowed from an unrelated convention.
  - `Config.UDPSize` and everything that threaded it through
    (`core/dnsserver`, `setup.go`'s `maxUDPMessageSize`) were removed
    entirely rather than kept alongside TCP: once genuinely large pushes
    go over TCP, no legitimate UPDATE traffic needs a bigger UDP receive
    buffer any more, and every test that previously needed it now sends
    over TCP instead (`handler_test.go`'s `serveThroughRealServer` binds
    both transports on the same port, matching a real deployment).
  - Verified against the real binary and manually against the real
    server that found this: a small partial update still goes out over
    UDP (confirmed in the server's own log); a full-zone push of the
    same shape that previously vanished now goes over TCP automatically
    and is accepted.
- **Fixed every RRSIG this package ever produced serving a wrong TTL of
  zero.** Found live against a real cutover: a real validating resolver
  (Unbound, with `aggressive-nsec` on) returned `SERVFAIL` for negative
  answers this server served, with no obvious cause -- positive answers
  from the same zone validated perfectly (`ad` bit set). Root-caused by
  building a real Unbound instance from scratch (Docker, full real root
  trust anchor, no shortcuts) against a locally reproduced copy of the
  exact same zone, isolating variable by variable: not the two-DS
  scenario (still failed with only the real key's own DS configured),
  not `aggressive-nsec` (still failed with it explicitly off) -- the
  actual cause was in `sign.go`'s `signOneRRset` the whole time.
  `miekg/dns`'s `RRSIG.Sign` sets `OrigTtl` (the RDATA field carried
  *inside* the signed data) but deliberately leaves `Hdr.Ttl` -- the
  RRSIG record's own wire TTL -- for the caller to set; `signOneRRset`
  never did, so every RRSIG this package has ever produced carried TTL 0,
  a direct violation of RFC 4034 §3 ("the TTL value of an RRSIG RR MUST
  match the TTL value of the RRset it covers"). The literal Unbound log
  line that gave it away: `TTL 0: dropped msg from cache` -- immediately
  discarding a just-received, validly-signed RRset from its own cache
  mid-validation corrupted its multi-step recursive validation state,
  surfacing as an opaque `SERVFAIL` (`Cannot retrieve DS for signature`)
  for answers that were otherwise completely valid. Fixed by setting
  `sig.Hdr.Ttl` from the covered RRset's own TTL before signing. Verified
  three ways: (1) the real Unbound reproduction above, both before (fails)
  and after (passes, `ad` bit set, TTLs correctly decrementing in cache)
  the fix, for both a NODATA answer and a positive one; (2) a new unit
  test (`TestSignZoneContentRRSIGsCarryTheCoveredRRsetsTTL`) asserting
  this permanently; (3) the full existing suite still green. This
  explains a real end-user report ("`dig` looks right, but `ping` fails")
  that had otherwise resisted diagnosis through several rounds of
  network-level investigation (packet captures, transport fixes) --
  those were real findings, but this was the actual root cause a strict
  validating resolver was reacting to the whole time.

- **Registration record: key + contact address together (§10.6).**
  A zone's contact address (used to alert on delegation changes, §11) rides
  an ordinary, already-authenticated UPDATE as a TXT RRset at a reserved
  owner name (`_sazu-contact.<zone>`), rather than a new wire-format field
  -- SIG(0) on the containing message already authenticates it, so no
  separate signature, transport, or protocol version bump was needed.
  `contact.go`'s `splitContactOps` strips it out of the ops before
  anything downstream (prerequisites, RRSIG verification, `ApplyUpdateOps`,
  the served zone) ever sees it as zone content: unlike a DNSKEY, a
  contact address has no reason to be public, queryable DNS data, and
  unlike zone content it is never itself DNSSEC-signed. Addresses are
  validated to a closed scheme set (`mailto:`, `http://`, `https://`) so
  `sazu-watchd` can later dispatch on scheme alone. Persisted in the
  existing (previously unused) `contacts` SQLite table, transactionally
  with the rest of `CommitUpdate`, and rehydrated by `LoadAll` into a new
  in-memory `ContactRegistry` alongside `Store`/`KeyRegistry`. `sazuctl
  contact` is the sanctioned client path (`-address`/`-clear`); it
  deliberately does *not* run the TXT through `SignZoneContent`, since a
  contact record needs no RRSIG of its own -- and `splitContactOps`
  defensively drops one anyway if a naively-built client sends it,
  so an orphan signature can never leak into served zone content.

- **Algorithm policy / weak-algorithm floor (§10.7).** A first-contact
  candidate DNSKEY whose algorithm RFC 8624 §3.1 rates MUST NOT or NOT
  RECOMMENDED for zone signing (RSAMD5, DSA/SHA1, RSASHA1,
  DSA-NSEC3-SHA1, RSASHA1-NSEC3-SHA1, RSASHA512, ECC-GOST, or anything
  unrecognized) is refused outright, with the `ERR_WEAK_ALGORITHM`
  diagnostic, before any cryptographic effort is spent verifying its
  SIG(0) -- `algorithm.go`'s `algorithmMeetsFloor` is a deliberate
  allowlist (RSASHA256, ECDSAP256SHA256, ECDSAP384SHA384, ED25519, ED448),
  not a denylist, so an unrecognized future algorithm number fails closed
  rather than being silently accepted. Only checked at first contact --
  once pinned, a key's algorithm can't change without a rollover (§10.4,
  still outstanding), so there's nothing new to check on a later ordinary
  push. This is the Go port's counterpart to the earlier Rust/rDNS port's
  `meets_minimum_floor()` check, which hadn't carried over until now.

- **Rate limiting / quota (§12).** Each zone gets two independent
  per-day quotas over a rolling (not calendar-day) 24h window: full-zone
  pushes and differential (`push-update`) ones, defaulting to the design
  doc's starting numbers (5 and 50) and overridable per-instance via the
  new `rate_limit FULL_PER_DAY DIFFERENTIAL_PER_DAY` Corefile directive.
  `ratelimit.go`'s `RateLimiter` classifies a push as full-zone if it
  carries a DNSKEY at the apex (true of every first-contact push, and of
  every full re-push, since `BuildFullZonePush` always re-asserts it) --
  the same signal that already distinguishes the two client-side
  subcommands (`push-zone` vs `push-update`). Checked right after SIG(0)
  verification, before the expensive first-contact chain-of-trust walk,
  so an already-exhausted quota doesn't also pay for that network round
  trip. An exceeded quota is refused with the new `ERR_QUOTA_EXCEEDED`
  diagnostic. Deliberately not persisted across a restart -- a purely
  advisory abuse/churn guard, not something a customer depends on for
  correctness, so the failure mode of losing quota history is "briefly
  too permissive," never "a customer locked out of their own zone."

- **Global, per-source-IP flood/scan throttle: `ERR_RATE_LIMITED`
  (§12).** Closes a real gap the per-zone quota above leaves open: that
  quota is keyed by zone name, so an attacker who varies the *target*
  zone name on every attempt -- "probe many candidate zones, looking for
  one whose delegation or DNSSEC state is exploitable" -- gets a fresh,
  entirely unused quota bucket every single time, no matter how many
  attempts they've already made. `ipratelimit.go`'s `IPRateLimiter`
  closes it with an address-keyed limit, independent of which zone(s) an
  address targets: a rolling 1-minute window (deliberately much shorter
  than the per-zone quota's 24h one -- this is a flood/scan guard
  reacting on the timescale a scan actually happens on, not a churn
  guard), defaulting to 30 UPDATE attempts/minute per source address and
  overridable via a new `ip_rate_limit UPDATES_PER_MINUTE` Corefile
  directive. Checked in `serveUpdate` before anything else -- before
  SIG(0) verification, even, and before the existing per-zone quota --
  deliberately: it bounds raw attempt *volume*, not just successfully
  authenticated attempts, since an attacker's failed probes cost this
  server real CPU (and, for a first-contact attempt, a real outbound
  chain-of-trust network walk) whether or not anything about the attempt
  ever turns out to verify. Shares its sliding-window compaction logic
  with `RateLimiter` (`slidingWindowAllow`) rather than duplicating it.
  Applies uniformly across every transport (UDP/TCP/HTTPS/HTTP3), same as
  the per-zone quota, since it's in the same shared `serveUpdate` code
  path. An exceeded limit is refused with the new `ERR_RATE_LIMITED`
  diagnostic -- §12's last remaining status code, now implemented.
  Deliberately not persisted across a restart, for the same reason
  `RateLimiter` isn't.

  Verified end to end (a real `dnsserver.Server`, real signed pushes)
  against the exact scenario this exists for: three real, distinct,
  never-before-seen zone names onboard normally from one address (each
  with its own, entirely untouched per-zone quota), and a fourth --
  still with a completely fresh per-zone quota of its own -- is refused
  purely on that address's accumulated attempt volume; also verified
  against a real, separately-built `coredns` binary and `sazuctl` the
  same way.

- **First-contact/rollover restricted to connection-oriented transports
  (`ERR_TRANSPORT_NOT_ALLOWED`), closing a spoofing bypass of the throttle
  above.** Found immediately after building `IPRateLimiter`: it's keyed
  by the apparent source address, but plain UDP has no handshake --
  a single forged packet can claim any source address at all, with
  nothing to disprove it. An attacker exploiting that could vary the
  (spoofed) source address on every packet and get a fresh per-address
  budget every time, each attempt still costing this server a real
  outbound chain-of-trust network query -- turning a rate limiter meant
  to bound exactly that cost into no protection at all. A real minimal
  attack packet for this is smaller than it might look: `containsAPEXSOA`
  is only checked *after* the chain-of-trust walk in `serveUpdate`, so
  the smallest message that reaches it is just a candidate DNSKEY plus a
  genuine SIG(0) signature -- no SOA, no RRSIGs, no NSEC chain -- comfortably
  under a single UDP datagram, unlike any *legitimate* push (which always
  includes at least a SOA and, from a real full push, an NSEC chain and
  RRSIGs, routinely well over 512 bytes as found elsewhere in this
  document).

  `handler.go`'s `connectionOriented` now gates the one first-contact/
  rollover branch that triggers that network walk on the transport
  actually being one where address spoofing doesn't work: TCP, or
  HTTPS/HTTP3 (both TLS-over-TCP and QUIC perform their own
  handshake-based address validation before any real work happens) --
  never plain UDP. Detecting HTTPS/HTTP3 checks `dnsserver.RawRequestKey`'s
  presence on the request context rather than the concrete `net.Addr`
  type `w.RemoteAddr()` returns, specifically because `ServerHTTPS3`
  constructs its `DoHWriter`'s address as a `*net.UDPAddr` (QUIC runs
  over UDP) -- indistinguishable from plain, spoofable UDP by address
  type alone, even though QUIC's own handshake makes an HTTP/3 request
  just as address-validated as TCP.

  No impact on any legitimate, already-documented workflow: a real
  first-contact push already always exceeds the plain-UDP size ceiling in
  practice (see the previous bullet and the RRSIG-TTL / TCP-transport
  findings elsewhere in this document), so every push this repo's own
  tooling (`sazuctl`) or test suite ever sends over UDP was already an
  ordinary, already-authenticated push to an already-pinned zone -- the
  one case this gate deliberately leaves untouched, since it never
  touches the chain-of-trust walk in the first place. Verified end to
  end: the exact minimal attack packet described above is refused over
  UDP, the identical push succeeds once resent over TCP, an equivalent
  rollover attempt over UDP is refused the same way, and an ordinary
  partial push to an already-pinned zone still works over UDP exactly as
  before.

- **`IPRateLimiter`/`RateLimiter` periodic garbage collection, closing a
  second gap found in the same pass.** Both limiters key their state by
  something an attacker can cheaply vary for free -- `IPRateLimiter` by
  source address (including a one-shot spoofed UDP address, by
  construction never seen twice), `RateLimiter` by zone name (a garbage
  candidate name costs nothing to invent) -- and neither previously ever
  removed a map entry once created, even after every timestamp in it
  aged out of the window. That meant either limiter's own memory usage
  grew with the number of *distinct keys ever attempted*, unboundedly,
  for the lifetime of the process -- a rate limiter that was itself an
  unbounded-memory attack surface. `ratelimit.go`'s new `sweepIfDue`
  garbage-collects every fully-stale entry across a limiter's bucket(s),
  at most once per that limiter's own window (24h for `RateLimiter`, 1
  minute for `IPRateLimiter`) so the amortized cost stays negligible.
  This bounds memory instead by "how many distinct keys were active in
  roughly the last window," itself bounded by an attacker's own
  sustained traffic rate -- an ordinary, expected property of a rate
  limiter, not a new attack surface. Verified directly: 500 distinct
  addresses (and, separately, 500 distinct zone names) each get their
  own entry, ages them all out of the window, and confirms the very next
  call -- which is what triggers a sweep -- collapses the map back down
  to just the one entry that triggered it.

- **Key rollover (§10.4).** An already-pinned zone can present a brand
  new candidate key -- no restart, no separate out-of-band step -- by
  sending a push signed by (and introducing) that new key. Implemented by
  reusing first contact's exact machinery rather than inventing parallel
  logic: `serveUpdate` tries the pinned key first, exactly as before; only
  if that verification fails does it look for a *different* candidate
  DNSKEY in the same ops and, if one both signs this transaction and
  passes the identical chain-of-trust-to-the-parent-DS check first
  contact requires, treats the push as a rollover -- re-pinning
  `KeyRegistry` (and, if configured, `DB`) to the new key. An ordinary
  push's failure mode (wrong key, corrupted signature) is completely
  unchanged: it only ever reaches the rollover branch after the pinned
  key has already failed, and only succeeds there if a genuinely distinct,
  self-verifying, DS-anchored candidate exists. Deliberately does *not*
  also require the *old* key's signature as a second factor: whoever can
  get a DS published at the registrar already fully controls the
  delegation regardless (that is the root of trust first contact itself
  already rests on), so requiring only that same proof for a rollover
  doesn't introduce a new attack surface beyond what first contact
  already accepts. The §10.7 algorithm floor applies to a rollover's new
  candidate exactly as it does at first contact, checked before any
  signature verification or chain-of-trust effort is spent on it. A
  rollover push doesn't need to re-establish a SOA (unlike true first
  contact) -- the zone already has real content from before, and a
  rollover may legitimately carry nothing but the new key itself.

  This also resolves the KSK/ZSK question this item used to carry
  alongside it: given rollover now exists, the argument for splitting
  the two (avoiding a DS/registrar update on every rotation) has a
  place to land, but SAZU's threat model -- the private key only ever
  signs offline, on the customer's own machine, never held by an
  always-on server -- still doesn't need the *other* traditional reason
  for the split (limiting exposure of a frequently-used online key).
  Decision: keep the single-key model (§9.1's own deliberate choice) for
  now; a KSK/ZSK split remains straightforward to add later on top of the
  rollover machinery built here, if a real customer workflow ever needs
  independent, more-frequent content re-signing without a registrar step.

- **Key custody hardening (§10.8), client-side.** `sazuctl` writes a plain
  BIND-format key file by default, unchanged -- but every subcommand that
  touches one now accepts `-key-passphrase-file <path>`: give it and that
  key file is encrypted at rest instead (scrypt-derived AES-256-GCM key;
  `keycrypt.go`), with no new external dependency (`golang.org/x/crypto`
  was already in this tree). The encrypted format is a fixed magic line
  plus a JSON envelope (KDF params, salt, nonce, ciphertext) wrapping the
  exact same BIND-format bytes `SavePrivateKey` would otherwise write, so
  decrypting one out-of-band still yields a file standard DNSSEC tooling
  can read. `LoadPrivateKey`/`LoadOrGenerateKey` detect and handle both
  formats transparently; a wrong or missing passphrase against an
  encrypted file fails the same way a corrupted file would (AES-GCM
  authentication), never with a distinguishable error. Real HSM/PKCS#11
  support -- holding the key in hardware, never as bytes on disk at all --
  remains a materially bigger, separate step (new dependency, a real or
  software HSM to test against, an API redesign for signing) and is left
  for if/when that's actually needed; this covers the much more common
  risk (a laptop or CI secret store gets compromised or synced somewhere
  it shouldn't) without it.

- **`ERR_STALE_SERIAL` and `ERR_EXPIRED_SIGNATURE` status codes (§12).**
  `EvaluatePrerequisites` now returns a status code alongside its rcode:
  `ERR_STALE_SERIAL` specifically for the SOA-serial staleness guard
  (`BuildFullZonePush`'s `previousSOA` parameter, an RFC 2136 §2.4.2
  value-dependent prerequisite against the apex SOA) failing, leaving
  every other, more generic prerequisite failure with no status code as
  before. `VerifySignedRRsets` similarly now distinguishes
  `ERR_EXPIRED_SIGNATURE` -- a covering RRSIG that is otherwise
  completely legitimate (right name, type, key tag, algorithm, and a
  cryptographically valid signature) but simply outside its own
  inception/expiration window -- from the more generic `ERR_SIG_INVALID`
  (no valid signature at all). The distinction matters operationally:
  one means "re-sign and re-push," the other means something is actually
  wrong with the key or the content.

- **Audit trail: transaction UUID, persistent log (§12).** Every §12
  status code from the design doc's list is now implemented (see the
  global per-source-IP flood throttle above, `ERR_RATE_LIMITED` -- the
  one still outstanding when this bullet was first written, since
  landed). `serveUpdate` now generates a fresh transaction ID (`audit.go`'s
  `newTransactionID`, a hand-rolled RFC 4122 v4 UUID -- no dependency
  needed for something this simple) and, when `db` is configured, writes
  one `audit_log` row per transaction *regardless of outcome* -- accepted
  or rejected, via a single exit point (`serveUpdate`'s local `reply`
  closure) so every one of its dozen-plus return paths is covered
  uniformly rather than needing its own explicit logging call. `zone` in
  `audit_log` is deliberately not a foreign key into `zones(origin)`,
  unlike every other table: a rejected first-contact attempt never
  creates a zones row at all, and that is exactly the kind of attempt an
  audit trail exists to remember. `DB.RecentTransactions` is the read
  side (newest first, optionally limited) for an operator asking "what
  happened to this zone's pushes recently." Deliberately server-side
  only for now: the transaction ID is not (yet) surfaced to the client
  on the wire, since doing so risked changing the Additional-section
  status-TXT contract `TestOnboardDeniedForOtherChainReasonsCarriesNoDiagnostic`
  depends on (no diagnostic TXT at all for a bare, generic rejection).
  The HTTPS/JSON carrier (below) would have more room to carry this --
  e.g. a `txid` field alongside the JSON wire envelope's response -- if
  this is ever revisited, but that's a distinct, smaller enhancement on
  top of an already-complete carrier, not a blocker for it.

- **§11 Delegation-change monitoring & alerting: `sazu-watchd`.** A
  standalone daemon (`plugin/sazu/cmd/sazu_watchd`), kept out of CoreDNS
  exactly as this document always intended: it's a periodic background
  job, not request-driven, and its own failure mode (a slow/flaky query
  to some TLD server) must never add latency to actual DNS answers or
  tie monitoring continuity to the query-serving process's uptime. On
  every tick (`-interval`, default 5m; `-once` for a single pass) it:
  1. Reads every onboarded zone's pinned key straight from the same
     SQLite file CoreDNS's `db` directive writes to (`DB.ListZones`/
     `LoadKey`, added alongside this rather than reusing the heavier
     `LoadAll`, since watchd never needs a zone's actual RR content).
  2. Re-runs `chain.go`'s `Validator` -- imported as a library, the exact
     same "does a DS matching this key exist at the parent" check first
     contact and a §10.4 key rollover already perform -- against each one.
  3. Compares against an in-memory "last known good" per zone
     (`watch.go`'s `checkOnce`); on a transition (OK→failing or
     failing→OK), alerts the zone's §10.6 registered contact
     (`DB.LoadContact`) and logs regardless of whether a contact is even
     registered. A zone's *first* observation only establishes a
     baseline and never alerts on its own -- there is no "last known" yet
     to have changed from, which is also what keeps daemon startup from
     alert-storming on every zone that happens to already be in a
     long-standing, already-known failure state.

  Alerting (`alert.go`'s `Notifier`) dispatches per-address on scheme,
  supporting both channels §11 left "TBD" between rather than picking
  one: `mailto:` via SMTP (stdlib `net/smtp`, optional auth, password
  supplied via `-smtp-password-file` for the same reason `sazuctl`'s own
  `-key-passphrase-file` avoids CLI-visible secrets) and `http(s)://` via
  a small self-describing JSON webhook POST. Zone state is deliberately
  not persisted across a `sazu-watchd` restart -- purely advisory
  monitoring continuity, not correctness-critical data, so the failure
  mode of losing it is "possibly miss one alert if a break-and-recover
  both happen within one restart window," never a false report.

  Verified against a real, live CoreDNS + `sazuctl` + `sazu-watchd`
  three-binary setup sharing one real SQLite file end to end (onboard a
  zone, register a contact, stop CoreDNS, run `sazu-watchd -once`,
  confirm via direct SQLite inspection that the zone/key/contact/audit
  rows it reads are exactly what CoreDNS wrote), in addition to its own
  unit tests (a fake `ChainValidator` for the state-transition logic,
  `httptest`/an unconfigured `Notifier` for alert dispatch).

- **HTTPS/JSON carrier, RFC 8427 (§7.3).** A push now works identically
  over HTTPS/HTTP3 as it already does over UDP/TCP, with two related
  CoreDNS-core gaps fixed first (`core/dnsserver`, its own commit,
  rebased in from a branch off `master` per this document's established
  workflow for pure-CoreDNS fixes):

  - `Config.AllowOpcode` reached UDP, TCP, and DNS-over-TLS only --
    HTTPS/HTTP3's `ServeHTTP` called `doh.RequestToMsgWire` unconditionally,
    which hardcodes the default, query-only accept policy (RFC 2136
    UPDATE is explicitly rejected by it, plus tight section-count limits
    meant for ordinary queries) with no way to honor a server's own
    opted-in opcodes. Fixed by threading an accept func through
    (`dnsutil.UnpackRequestWithAcceptFunc`, `doh.RequestToMsgWireWithAccept`),
    and having both `ServerHTTPS`/`ServerHTTPS3` pass their own
    `s.msgAcceptFunc()` -- the exact same one UDP/TCP/TLS already use --
    instead of `nil`.
  - Even with that fixed, `doh.RequestToMsgWire` extracts the raw wire
    bytes internally but never handed them to a plugin -- unlike UDP/TCP,
    which do via `UDPDecorateReaderFunc`/`TCPDecorateReaderFunc`. Fixed by
    a new `core/dnsserver.RawRequestKey` context value, set by both
    `ServeHTTP` methods alongside the existing `HTTPRequestKey`.
  - Also added, in the same core change: `doh.JSONWireEnvelope`
    (`{"wire": "<base64>"}`, `Content-Type: application/dns-message+json`
    or generic `application/json`) as an explicitly-scoped, non-RFC-8484
    extension -- deliberately *not* a structural (RFC 8427) translation
    of the message's fields, since SIG(0) signs literal wire bytes and
    there is no lossless mapping back from parsed JSON fields to that
    exact byte sequence. Wrapping the same raw bytes in base64 has no
    such problem. `RawRequestKey` carries the *decoded* bytes either way,
    so a plugin never needs to know which carrier a request used.

  On the sazu side, this needed exactly one change: `serveUpdate` now
  falls back to `ctx.Value(dnsserver.RawRequestKey{})` when
  `s.Capture.Take` finds nothing (i.e., the request didn't arrive over
  UDP/TCP at all). Everything downstream -- SIG(0) verification, chain of
  trust, prerequisites, RRSIG checks, persistence, the audit trail -- is
  the exact same code every other transport already shares; `setup.go`
  needed no changes at all, since it already calls
  `config.AllowOpcode(dns.OpcodeUpdate)` on whatever transport's `Config`
  a Corefile's server block hands it. `sazuctl` gained matching client
  support: every push-capable subcommand sends over HTTPS instead of
  UDP/TCP when `-target` is an `http(s)://` URL, with `-json` selecting
  the JSON envelope over raw bytes.

  Verified two ways: a real `dnsserver.ServerHTTPS` driven directly in
  this package's own tests (a genuine SIG(0)-signed first-contact push,
  in both raw-bytes and JSON-envelope form, plus an ordinary
  already-pinned-key push afterward), and a real, separately-built
  `coredns` binary with a real `https://` server block (self-signed
  cert, real TLS handshake) pushed to by a real `sazuctl` binary in both
  forms.

## Outstanding

Nothing. Every item the architectural review that led to this document
identified -- CoreDNS-plugin, client-side, and the one separate-server
item -- is implemented; see **Done**, above. Most recently: the global
per-source-IP flood/scan throttle (`ERR_RATE_LIMITED`) and the
connection-oriented-transport requirement for first contact/rollover
that closes a spoofing bypass of it, both found and fixed in the same
pass while hardening the HTTPS/JSON carrier (§7.3) area.
