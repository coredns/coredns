# dnslkg

## Name

*dnslkg* - serves the Last Known Good (LKG) DNS answer when an upstream returns
NXDOMAIN, NODATA or an error.

## Description

The *dnslkg* plugin persists every successful answer it observes into an on-disk
SQLite database. When the upstream subsequently returns a negative response
(NXDOMAIN or NODATA), an error response (e.g. SERVFAIL) or fails to respond at
all, *dnslkg* replies with the previously stored answer instead - the *last
known good* result.

This protects against outages caused by an upstream that is **healthy but
misconfigured** - for example a bug in a provisioning system that publishes an
empty or wrong zone, causing names that used to resolve to suddenly return
NXDOMAIN/NODATA. This class of failure (as seen in large scale cloud DNS
incidents) is *not* covered by the *cache* plugin's `serve_stale` option, which
only serves stale data while the upstream is considered **unhealthy** and which
keeps its data in memory only (lost on restart and not shared across processes).

Because the store is on disk, last known good answers survive CoreDNS restarts.
SQLite is opened in WAL mode with a busy timeout, providing high-performance,
concurrent, persistent, in-process storage.

The set of names handled by the plugin can be narrowed with `include` and
`exclude` regular expressions. With no patterns configured, all names are
tracked.

Only names that have previously produced a *good* answer of the *same query
type* can be served from the store, so a genuine NODATA for a type that never
existed (e.g. an `AAAA` for an IPv4-only host) is passed through untouched.

## Syntax

~~~ txt
dnslkg [DBPATH]
~~~

* **DBPATH** is the path to the SQLite database file. Defaults to `dnslkg.db`
  in the CoreDNS working directory.

The extended syntax allows finer control:

~~~ txt
dnslkg [DBPATH] {
    path    DBPATH
    ttl     DURATION
    include REGEX...
    exclude REGEX...
}
~~~

* `path` **DBPATH** sets the SQLite database file location (alternative to the
  inline argument).
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

Serve last known good answers for every name, storing the database at the given
path:

~~~ corefile
. {
    dnslkg dnslkg.db
    cache
    forward . 8.8.8.8
}
~~~

Only protect a couple of critical domains, and never protect an internal test
domain:

~~~ corefile
. {
    dnslkg dnslkg.db {
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
