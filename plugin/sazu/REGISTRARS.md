# Publishing a DS record at your registrar

**Status: placeholder.** This document doesn't have registrar-specific
walkthroughs yet — it exists so `sazuctl`'s onboarding-denied message has
somewhere real to point to, and so the per-registrar instructions have an
obvious home once they're written. If your registrar isn't listed below,
search their support site for "DS record," "DNSSEC," or "delegation
signer," or contact their support directly — every registrar that supports
DNSSEC has *some* way to do this, the interface just varies.

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

## Registrar-specific notes (TODO)

The design document (`sazu-protocol.md` §10.3, in the separate `sazu`
design repo) flags a few registrars with open questions worth confirming
empirically and writing up here:

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
