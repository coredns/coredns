# local

## Name

*local* - respond to local names.

## Description

*local* will respond with a basic reply to a "local request". Local requests are defined to be
names in the following zones: localhost, 0.in-addr.arpa, 127.in-addr.arpa and 255.in-addr.arpa,
any query under `.localhost.`, and, by default for backward compatibility, any query prefixed by
`localhost.`. When seeing one of the non-apex localhost forms a metric counter is increased and if
*debug* is enabled a debug log is emitted.

With *local* enabled any query falling under these zones will get a reply. This prevents the query
from "escaping" to the internet and putting strain on external infrastructure.

The zones are mostly empty, only `localhost.`, names under `.localhost.`, and legacy
`localhost.<domain>` names return loopback address records (A and AAAA), and only
`1.0.0.127.in-addr.arpa.` has a reverse (PTR) record.

## Syntax

~~~ txt
local
~~~

~~~ txt
local {
    localhost_prefix on|off
}
~~~

`localhost_prefix` controls the legacy `localhost.<domain>` behavior. The default is `on` for
backward compatibility. Set it to `off` to only treat names under `.localhost.` as special, which
matches RFC 6761. The legacy prefix behavior is deprecated and may be disabled by default in a
future release.

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metric is exported:

* `coredns_local_localhost_requests_total{}` - a counter of the number of non-apex localhost
  special-case queries CoreDNS has seen. This includes `.localhost.` names and, when
  `localhost_prefix` is `on`, legacy `localhost.<domain>` names. It does *not* count `localhost.`
  queries.

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
