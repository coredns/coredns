# Publishing a DS record at your registrar

**Status: in progress.** Only AWS Route 53 is confirmed so far; everything
else below is an open TODO. This exists so `sazuctl`'s onboarding-denied
message has somewhere real to point to, and so each registrar's
instructions have an obvious home once they're written. If your registrar
isn't listed below, search their support site for "DS record," "DNSSEC,"
or "delegation signer," or contact their support directly — every
registrar that supports DNSSEC has *some* way to do this, the interface
just varies.

## What you're doing, in general

Regardless of registrar, you're taking the output of:

```
sazuctl ds -zone yourdomain.example -key client.private
```

— a key tag, algorithm (15 = Ed25519), digest type (2 = SHA-256), and a
64-character hex digest — and entering those four values into your
registrar's DS-record / DNSSEC page. Most registrars ask for exactly those
four fields, sometimes labeled slightly differently ("Key Tag," "Flags" or
"Algorithm," "Digest Type," "Digest" or "Public Key"). None of them need
your private key, the zone content, or anything else — the DS record is
public data by design.

After submitting it, propagation is normally minutes to a few hours. You
can check whether it's live with:

```
dig DS yourdomain.example +short
```

## Migrating an already-live domain

**If this domain currently serves real traffic and has never had DNSSEC
enabled before, read this before submitting anything above.** A brand-new
domain with no live traffic yet has none of this risk — skip to
**Confirmed registrars** below.

Publishing a DS record makes every DNSSEC-validating resolver in the world
expect a signed answer for this domain *immediately* — not only once this
SAZU server actually becomes authoritative for it. Until that cutover is
complete, the domain's *current* host is still the one answering queries,
and if it isn't serving signatures matching the DS you just published, the
**entire domain** — not just DNSSEC-specific lookups — starts failing
(`SERVFAIL`) for every validating resolver, for as long as the mismatch
lasts. This is exactly what `sazuctl` calls out as `ERR_UNKNOWN_SIGNER`
and `ERR_NO_DS_PUBLISHED` guidance when it detects the relevant states.

To migrate safely:

1. **If your current host supports enabling its own DNSSEC signing, turn
   that on first**, and confirm the domain still resolves correctly
   everywhere (e.g. with an external DNSSEC-checking tool, or `dig +dnssec`
   against a validating resolver like 8.8.8.8 or 1.1.1.1). This keeps the
   domain validly signed under its *current* host's own key throughout the
   migration — there is no gap where a DS is published with nothing
   matching it.
2. **Only once you are actually ready to cut authoritative service over to
   this server**, replace that DS record with the one `sazuctl ds` prints
   for your SAZU key (the normal onboarding flow above) — ideally as close
   as possible to the moment the delegation/NS records also switch, since
   the DS and what's actually being served need to agree throughout.
3. **If your current host has no way to enable DNSSEC at all**, you don't
   have a safe way to keep this domain validated during a transition
   window on that host. Consider moving this domain's DNS hosting to one
   that does support it (AWS Route 53 is confirmed to, see below) *before*
   publishing any DS record for the SAZU key.

## Confirmed registrars

### AWS Route 53

Route 53's "Add a public key" flow for DNSSEC doesn't ask for a DS record
directly -- it asks for the DNSKEY's own fields and computes the DS
itself. Two of those fields are a dropdown of numeric codes rather than
free text, so here's exactly what to pick for a SAZU-generated key:

- **Public key type** -- Route 53 offers 256 (ZSK) or 257 (KSK). SAZU
  always generates SEP-flagged keys (the design's single-key model: one
  key both signs and authenticates, so it's always a KSK by DNSSEC's own
  definition of that flag), so pick **257 (KSK)**. `sazuctl keygen` and
  `sazuctl ds` both print this as "key type" / "public key type" so you
  don't have to work it out by hand.

- **Algorithm** -- Route 53's dropdown lists 2 (DH), 3 (DSA), 5
  (RSASHA1), 6 (DSA-NSEC3-SHA1), 7 (RSASHA1-NSEC3-SHA1), 8 (RSASHA256),
  10 (RSASHA512), 13 (ECDSAP256SHA256, **Route 53's own default**), 14
  (ECDSAP384SHA384), 15 (Ed25519), 16 (Ed448), 253 (PRIVATEDNS), and 254
  (PRIVATEOID). SAZU generates Ed25519 keys, so pick **15 (Ed25519)** --
  **do not leave this on Route 53's default of 13.** If you do, Route 53
  computes a DS digest under the wrong algorithm number: not
  `ERR_NO_DS_PUBLISHED` (a DS record *is* there), but `ERR_UNKNOWN_SIGNER`
  ("a DS record is published, but not for this key"), since the published
  digest no longer corresponds to your actual key at all. If onboarding
  fails with that specific diagnostic after using this flow, this
  mismatch is the first thing to check — though also see **Migrating an
  already-live domain** above, since a pre-existing, unrelated DS is
  another real cause of the same diagnostic.

- **Public key** -- the base64 value `sazuctl keygen`/`sazuctl ds` prints
  as "public key." Paste it exactly as shown; it's the same value either
  command prints, regardless of which one you ran.

Route 53 computes and publishes the DS record itself from these three
values, so there's nothing further to copy from `sazuctl ds`'s DS-record
line for this particular registrar.

## Registrar-specific notes (TODO)

The design document (`sazu-protocol.md` §10.3, in the separate
[github.com/mrwiora/sazu](https://github.com/mrwiora/sazu) repo) flags a
few registrars with open questions worth confirming empirically and
writing up here:

- [ ] **GoDaddy** — has a DS-record submission flow; whether it does a
  live-match check against the zone before accepting is unconfirmed.
- [ ] **Gandi** — has a DS-record submission flow; whether it does a
  live-match check like GoDaddy's is unconfirmed.
- [ ] **IONOS** — DS records for externally-hosted nameservers reportedly
  go through an email-based process; turnaround time is unconfirmed.
- [ ] **Cloudflare Registrar**, **Namecheap**, **Porkbun**, **OVH** — not
  yet investigated at all.

If you go through this process with a real domain, the single most useful
thing you can add here is: which registrar, how many clicks, how long
propagation actually took, and anything that wasn't obvious from their UI.
