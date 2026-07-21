# dnslkg

## Name

*dnslkg* - serves the Last Known Good (LKG) DNS answer when an upstream returns
NXDOMAIN, NODATA or an error.

## Description

The *dnslkg* plugin remembers every successful answer it observes. When the
upstream subsequently returns a negative response (NXDOMAIN or NODATA), an error
response (e.g. SERVFAIL) or fails to respond at all, *dnslkg* replies with the
previously stored answer instead - the *last known good* result.

This protects against outages caused by an upstream that is **healthy but
misconfigured** - for example a bug in a provisioning system that publishes an
empty or wrong zone, causing names that used to resolve to suddenly return
NXDOMAIN/NODATA. This class of failure (as seen in large scale cloud DNS
incidents) is *not* covered by the *cache* plugin's `serve_stale` option, which
only serves stale data while the upstream is considered **unhealthy**.

Which failures trigger a fallback is configurable with `fallback_on`, so the
plugin can be as aggressive (mask authoritative NXDOMAIN/NODATA) or as
conservative (only cover SERVFAIL/timeouts) as an operator wants.

The store is a simple, bounded, in-memory map with no external dependencies. It
is capped at `max_entries` records; when full, the least-recently-written entry
is evicted, so memory use stays bounded regardless of how many distinct names
are queried. Reads take a shared lock and never mutate shared state, so
concurrent look-ups do not contend. There is no background goroutine and no disk
I/O. Because the store is in memory only, entries do **not** survive a CoreDNS
restart; the `Store` interface is designed so a persistent backend can be added
later.

Only names that have previously produced a *good* answer of the *same query
type* can be served from the store, so a genuine NODATA for a type that never
existed (e.g. an `AAAA` for an IPv4-only host) is passed through untouched.

## Syntax

~~~ txt
dnslkg
~~~

The extended syntax allows finer control:

~~~ txt
dnslkg {
    fallback_on      TRIGGER...
    max_age          DURATION
    fallback_timeout DURATION
    ttl              DURATION
    max_entries      N
    include          PATTERN...
    exclude          PATTERN...
}
~~~

* `fallback_on` **TRIGGER...** selects which upstream failures cause the stored
  answer to be served. Valid triggers are `nxdomain`, `nodata`, `timeout` and
  `error` (with `all`, `none` and the `servfail` alias for `error`). Defaults to
  `nxdomain nodata timeout error` (all). For example, `fallback_on nodata` only
  masks NODATA responses and passes real NXDOMAIN/SERVFAIL through. Note that
  answers are only ever *stored* from successful responses; this option controls
  *serving* only.
* `max_age` **DURATION** is the maximum age of an entry that may be served.
  Entries older than this are treated as absent and reclaimed. Defaults to `0`
  (no age limit).
* `fallback_timeout` **DURATION** turns on "verify" mode: the query is always
  forwarded upstream, but if no answer arrives within this window the stored
  answer is served immediately (the late upstream answer still refreshes the
  store). Only active when `timeout` is one of the `fallback_on` triggers.
  Defaults to `0` (disabled - wait for the upstream normally).
* `ttl` **DURATION** is the TTL stamped on records of answers served from the
  store. A short value (default `30s`) is recommended so clients re-query
  frequently and pick up a recovered upstream quickly. Set to `0s` to instead
  serve the original TTLs decremented by the age of the stored entry.
* `max_entries` **N** is the maximum number of answers held in memory. Must be a
  positive integer. Defaults to `10000`.
* `include`/`exclude` **PATTERN...** select which names are tracked, using
  wildcard domain patterns (see below). May each be specified multiple times.

### Name selection

`include`/`exclude` patterns are wildcard domains:

* `example.com` - matches the apex `example.com.` only.
* `*.example.com` - matches any subdomain (`a.example.com`, `a.b.example.com`),
  but **not** the apex. Add `example.com` as well to cover the apex. The `*`
  wildcard is only valid as the leftmost label.

Overlapping rules are resolved **most-specific-wins** (independent of the order
they are written): the rule matching the most labels wins; at equal depth an
exact pattern beats a wildcard; on a true tie `exclude` wins. If any `include`
rule is present, a name matching no rule is **not** tracked (allow-list); with
only `exclude` rules, unmatched names **are** tracked (deny-list). With no rules
at all, every name is tracked. This makes it easy to compose rules, e.g. track a
whole zone, carve out a subtree, then re-include one name:

~~~ txt
include *.example.com
exclude *.internal.example.com
include api.internal.example.com
~~~

## Metrics

If monitoring is enabled (via the *prometheus* plugin) the following metrics are
exported:

* `coredns_dnslkg_stored_total{server}` - the count of upstream answers stored
  as last known good.
* `coredns_dnslkg_served_total{server}` - the count of responses served from the
  last known good store.

## Examples

Serve last known good answers for every name (aggressive default):

~~~ corefile
. {
    dnslkg
    cache
    forward . 8.8.8.8
}
~~~

Conservative: only cover SERVFAIL/timeouts, verify with a 200ms soft deadline,
and expire entries after a day:

~~~ corefile
. {
    dnslkg {
        fallback_on error timeout
        fallback_timeout 200ms
        max_age 24h
    }
    cache
    forward . 8.8.8.8
}
~~~

Protect a couple of critical domains, never protect an internal subtree, cap the
store at 1000 entries:

~~~ corefile
. {
    dnslkg {
        max_entries 1000
        ttl 15s
        include *.example.com *.example.org
        exclude *.internal.example.com
    }
    cache
    forward . 8.8.8.8
}
~~~

## See Also

The *cache* plugin's `serve_stale` option, and the
[Azure DNS Client Cache (DNS LKG)](https://learn.microsoft.com/azure/dns/)
feature that inspired this plugin.