# local

## Name

*local* - respond to local names.

## Description

*local* will respond with a basic reply to a "local request". Local requests are defined to be
names in the following zones: localhost, 0.in-addr.arpa, 127.in-addr.arpa and 255.in-addr.arpa *and*
any query under `.localhost.`. When seeing the latter a metric counter is increased and if *debug*
is enabled a debug log is emitted.

With *local* enabled any query falling under these zones will get a reply. This prevents the query
from "escaping" to the internet and putting strain on external infrastructure.

The zones are mostly empty, only `localhost.` and names under `.localhost.` return loopback address
records (A and AAAA), and only `1.0.0.127.in-addr.arpa.` has a reverse (PTR) record.

## Syntax

~~~ txt
local
~~~

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metric is exported:

* `coredns_local_localhost_requests_total{}` - a counter of the number of queries under `.localhost.`
  requests CoreDNS has seen. Note this does *not* count `localhost.` queries.

Note that this metric *does not* have a `server` label, because it's more interesting to find the
client(s) performing these queries than to see which server handled it. You'll need to inspect the
debug log to get the client IP address.

## Examples

~~~ corefile
. {
    local
}
~~~

## Bugs

Only the `in-addr.arpa.` reverse zone is implemented, `ip6.arpa.` queries are not intercepted.

## See Also

BIND9's configuration in Debian comes with these zones preconfigured. See the *debug* plugin for
enabling debug logging.
