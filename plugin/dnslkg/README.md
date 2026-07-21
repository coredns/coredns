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

The store is a simple, bounded, in-memory map with no external dependencies. It
is capped at `max_entries` records; when full, the least-recently-written entry
is evicted, so memory use stays bounded regardless of how many distinct names
are queried. Reads (only on the upstream-failure path) take a shared lock and
never mutate shared state, so concurrent look-ups do not contend. There is no
background goroutine and no disk I/O. Because the store is in memory only, the
last known good answers do **not** survive a CoreDNS restart; the `Store`
interface is designed so a persistent backend can be added later.

The set of names handled by the plugin can be narrowed with `include` and
`exclude` regular expressions. With no patterns configured, all names are
tracked.

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
    max_entries N
    ttl         DURATION
    include     REGEX...
    exclude     REGEX...
}
~~~

* `max_entries` **N** is the maximum number of answers held in memory. Must be a
  positive integer. Defaults to `10000`.
* `ttl` **DURATION** is the TTL stamped on records of answers served from the
  store. A short value (default `30s`) is recommended so clients re-query
  frequently and pick up a recovered upstream quickly. Set to `0s` to instead
  serve the original TTLs decremented by the age of the stored entry.
* `include` **REGEX...** only names matching at least one of these regular
  expressions are tracked. May be specified multiple times.
* `exclude` **REGEX...** names matching any of these regular expressions are
  never tracked, even if they also match an `include` pattern. May be specified
  multiple times.

Regular expressions are matched against the lower-cased, fully-qualified query
name (e.g. `www.example.org.`).

## Metrics

If monitoring is enabled (via the *prometheus* plugin) the following metrics are
exported:

* `coredns_dnslkg_stored_total{server}` - the count of upstream answers stored
  as last known good.
* `coredns_dnslkg_served_total{server}` - the count of responses served from the
  last known good store.

## Examples

Serve last known good answers for every name:

~~~ corefile
. {
    dnslkg
    cache
    forward . 8.8.8.8
}
~~~

Only protect a couple of critical domains, never protect an internal test
domain, and cap the store at 1000 entries:

~~~ corefile
. {
    dnslkg {
        max_entries 1000
        ttl 15s
        include (^|\.)example\.com\.$ (^|\.)example\.org\.$
        exclude (^|\.)test\.internal\.$
    }
    cache
    forward . 8.8.8.8
}
~~~

## See Also

The *cache* plugin's `serve_stale` option, and the
[Azure DNS Client Cache (DNS LKG)](https://learn.microsoft.com/azure/dns/)
feature that inspired this plugin.