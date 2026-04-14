# dso

## Name

*dso* - implements [RFC 8490][rfc8490] DNS Stateful Operations and [RFC 8765][rfc8765] DNS Push Notifications.

## Description

The *dso* plugin allows clients to establish DNS Stateful Operations sessions. The service runs alongside
the DNS server of its server block, on its own ports (TCP and/or TLS). It reuses the listen hosts of that
server block, so *bind* applies to it as well and only the port differs.

A DSO service belongs to exactly one DNS server. Since a single DNS server may be defined by several server
blocks, the service is configured in one of them, and the remaining blocks repeat `dso` with no block of
their own so that traffic to any of them pairs the service, see [Examples](#examples).

If `push` is enabled, [RFC 8765][rfc8765] DNS Push Notification service is provided over TLS that notifies
subscribed clients about changes in resource records. The plugin then also responds authoritatively to queries
received over a session for names within the push zones; names outside them are answered with NOTAUTH.
Without `push` such queries are refused. TLS and TSIG configurations are taken from [*tls*][tls] and [*tsig*][tsig]
respectively, see [Bugs](#bugs).


Discovery of DSO services is performed by clients and is outside of the plugin's responsibilities.
Typically for DNS Push Notifications the main DNS server must respond to *_dns-push-tls._tcp.&lt;ZONE&gt;* `SRV` query.

## Syntax

Bare form:

~~~ txt
dso
~~~
The bare form installs the handler only and starts no service. It supplements the full form which must still be defined
but at least once server block of a DNS server.

Full form:

~~~ txt
dso {
    tcp_port PORT
    tls_port PORT
    keepalive KEEPALIVE [INACTIVITY]
    reconnect ONRESTART ONSHUTDOWN
    log
    push [ZONES...] {
        classes CLASS...
        types TYPES...
        refresh DURATION
        debounce DURATION
    }
}
~~~

* `tcp_port` **PORT** and `tls_port` **PORT** are the ports the service listens on. At least one of the two must be specified.
* `keepalive` **KEEPALIVE** [**INACTIVITY**] sets keep alive and inactivity timeouts. **KEEPALIVE** must be
  at least 10s. If **INACTIVITY** is omitted, it is set to **KEEPALIVE**.
  Since server is a passive observer, requests for shorter timeouts are granted as it allows to shed dead
  connections sooner.
  Defaults to 1h and 1m respectively.
* `reconnect` **ONRESTART** **ONSHUTDOWN** is the retry delay sent to clients when CoreDNS configuration is
  reloaded or the process is gracefully shut down.
  Defaults to 5s and 15s respectively.
* `log` enables use of CoreDNS logging system to print incoming and outgoing DSO messages. If the *debug* plugin is enabled,
  malformed messages are hex dumped.
* `push` [**ZONES...**] enables [RFC 8765][rfc8765] DNS Push Notifications; requires `tls_port`.
  **ZONES** are the zones subscriptions are allowed for. If empty, the zones from the server block are used.
* `push.classes` **CLASS...** configures list of RR classes that *Class: ANY* subscriptions resolve to.
  Defaults to `IN`.
* `push.types` **TYPE...** configures list of RR types that *Type: ANY* subscriptions resolve to.
  Defaults to `A`, `AAAA`, `PTR`, `TXT` and `SRV`.
* `push.refresh` **DURATION** sets delay between lookups.
  Defaults to 1m, set to `0` to disable.
* `push.debounce` **DURATION** tames subscription burst by waiting before performing the first lookup.
  Defaults to 1s, set to `0` to disable.

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metrics are exported:

* `coredns_dso_session_duration_seconds{server}` - histogram of session durations.
* `coredns_dso_push_subscription_entries{server,rrtype}` - if `push` is enabled, number of active push subscriptions.
* `coredns_dso_push_subscription_requests_total{server,rrtype}` - if `push` is enabled, number of push subscription requests.
* `coredns_dso_push_subscription_hits_total{server,rrtype}` - if `push` is enabled, number of successful push subscription requests.

## Examples

Minimal DSO service:

~~~ corefile
example.org {
    dso {
        tcp_port 8053
    }
}
~~~

Minimal DSO service with DNS Push Notifications:

~~~ corefile
example.org {
    dso {
        tls_port 8853
        push
    }
}
~~~

Minimal DSO service for a DNS server defined by two server blocks:

~~~ corefile
example.org {
    dso {
        tls_port 8853
        push example.org example.net
    }
}

example.net {
    dso
}
~~~

## Limitations

DSO service is deferred until handler installed by the plugin sees at least one message from its parent DNS server.

CoreDNS does not (and often cannot) notify plugins of changes in the zones. Thus Push must periodically poll upstream.
These lookup queries can be distinguished as their local address is set to DSO's endpoint.

Since a DSO service belongs to exactly one DNS server, the Corefile is constrained in three ways:

* `dso` can be used at most once per server block.
* All keys of a server block using `dso` must listen on the same port.
* At most one server block per DNS server may carry a `dso` configuration block; the others must use the
  bare `dso` form.

Queries carried over a session are accepted more strictly than queries to the DNS server: only questions that
can produce resource record of desired type are answered. E.g. `AXFR`, `IXFR` question types are rejected,
so zone transfer over a DSO session is not possible.

## Bugs

Per [RFC 8765][rfc8765], push server must respond to DNS queries for served zones. However, it's possible to define
a plugin stack such that additional information present in client's DNS queries may result in answers that differ
from push lookups.

The plugin only sees *tsig* configuration in its immediate block.

## See Also

[RFC 8490][rfc8490] for DNS Stateful Operations and [RFC 8765][rfc8765] for DNS Push Notifications.
See the [*tls*][tls] plugin for configuring certificates and the [*tsig*][tsig] plugin for configuring TSIG secrets.

[rfc8490]: https://www.rfc-editor.org/rfc/rfc8490.html
[rfc8765]: https://www.rfc-editor.org/rfc/rfc8765.html
[tls]: https://coredns.io/plugins/tls
[tsig]: https://coredns.io/plugins/tsig
