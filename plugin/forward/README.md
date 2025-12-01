# forward

## Name

*forward* - facilitates proxying DNS messages to upstream resolvers.

## Description

The *forward* plugin re-uses already opened sockets to the upstreams. It supports UDP, TCP and
DNS-over-TLS and uses in band health checking.

When it detects an error a health check is performed. This checks runs in a loop, performing each
check at a *0.5s* interval for as long as the upstream reports unhealthy. Once healthy we stop
health checking (until the next error). The health checks use a recursive DNS query (`. IN NS`)
to get upstream health. Any response that is not a network error (REFUSED, NOTIMPL, SERVFAIL, etc)
is taken as a healthy upstream. The health check uses the same protocol as specified in **TO**. If
`max_fails` is set to 0, no checking is performed and upstreams will always be considered healthy.

When *all* upstreams are down it assumes health checking as a mechanism has failed and will try to
connect to a random upstream (which may or may not work).

## Syntax

In its most basic form, a simple forwarder uses this syntax:

~~~
forward FROM TO...
~~~

* **FROM** is the base domain to match for the request to be forwarded. Domains using CIDR notation
  that expand to multiple reverse zones are not fully supported; only the first expanded zone is used.
* **TO...** are the destination endpoints to forward to. The **TO** syntax allows you to specify
  a protocol, `tls://9.9.9.9` or `dns://` (or no protocol) for plain DNS. The number of upstreams is
  limited to 15.

Multiple upstreams are randomized (see `policy`) on first use. When a healthy proxy returns an error
during the exchange the next upstream in the list is tried.

Extra knobs are available with an expanded syntax:

~~~
forward FROM TO... {
    except IGNORED_NAMES...
    force_tcp
    prefer_udp
    expire DURATION
    max_fails INTEGER
    tls CERT KEY CA
    tls_servername NAME
    policy random|round_robin|sequential
    health_check DURATION [no_rec] [domain FQDN]
    max_concurrent MAX
    next RCODE_1 [RCODE_2] [RCODE_3...]
    failfast_all_unhealthy_upstreams
}
~~~

* **FROM** and **TO...** as above.
* **IGNORED_NAMES** in `except` is a space-separated list of domains to exclude from forwarding.
  Requests that match none of these names will be passed through.
* `force_tcp`, use TCP even when the request comes in over UDP.
* `prefer_udp`, try first using UDP even when the request comes in over TCP. If response is truncated
  (TC flag set in response) then do another attempt over TCP. In case if both `force_tcp` and
  `prefer_udp` options specified the `force_tcp` takes precedence.
* `max_fails` is the number of subsequent failed health checks that are needed before considering
  an upstream to be down. If 0, the upstream will never be marked as down (nor health checked).
  Default is 2.
* `expire` **DURATION**, expire (cached) connections after this time, the default is 10s.
* `tls` **CERT** **KEY** **CA** define the TLS properties for TLS connection. From 0 to 3 arguments can be
  provided with the meaning as described below

  * `tls` - no client authentication is used, and the system CAs are used to verify the server certificate
  * `tls` **CA** - no client authentication is used, and the file CA is used to verify the server certificate
  * `tls` **CERT** **KEY** - client authentication is used with the specified cert/key pair.
    The server certificate is verified with the system CAs
  * `tls` **CERT** **KEY**  **CA** - client authentication is used with the specified cert/key pair.
    The server certificate is verified using the specified CA file

* `tls_servername` **NAME** allows you to set a server name in the TLS configuration; for instance 9.9.9.9
  needs this to be set to `dns.quad9.net`. Multiple upstreams are still allowed in this scenario,
  but they have to use the same `tls_servername`. E.g. mixing 9.9.9.9 (QuadDNS) with 1.1.1.1
  (Cloudflare) will not work. Using TLS forwarding but not setting `tls_servername` results in anyone
  being able to man-in-the-middle your connection to the DNS server you are forwarding to. Because of this,
  it is strongly recommended to set this value when using TLS forwarding.
* `policy` specifies the policy to use for selecting upstream servers. The default is `random`.
  * `random` is a policy that implements random upstream selection.
  * `round_robin` is a policy that selects hosts based on round robin ordering.
  * `sequential` is a policy that selects hosts based on sequential ordering.
* `health_check` configure the behaviour of health checking of the upstream servers
  * `<duration>` - use a different duration for health checking, the default duration is 0.5s.
  * `no_rec` - optional argument that sets the RecursionDesired-flag of the dns-query used in health checking to `false`.
    The flag is default `true`.
  * `domain FQDN` - set the domain name used for health checks to **FQDN**.
    If not configured, the domain name used for health checks is `.`.
* `max_concurrent` **MAX** will limit the number of concurrent queries to **MAX**.  Any new query that would
  raise the number of concurrent queries above the **MAX** will result in a REFUSED response. This
  response does not count as a health failure. When choosing a value for **MAX**, pick a number
  at least greater than the expected *upstream query rate* * *latency* of the upstream servers.
  As an upper bound for **MAX**, consider that each concurrent query will use about 2kb of memory.
* `next` If the `RCODE` (i.e. `NXDOMAIN`) is returned by the remote then execute the next plugin. If no next plugin is defined, or the next plugin is not a `forward` plugin, this setting is ignored
* `failfast_all_unhealthy_upstreams` - determines the handling of requests when all upstream servers are unhealthy and unresponsive to health checks. Enabling this option will immediately return SERVFAIL responses for all requests. By default, requests are sent to a random upstream.

Also note the TLS config is "global" for the whole forwarding proxy if you need a different
`tls_servername` for different upstreams you're out of luck.

On each endpoint, the timeouts for communication are set as follows:

* The dial timeout by default is 30s, and can decrease automatically down to 1s based on early results.
* The read timeout is static at 2s.

## Metadata

The forward plugin will publish the following metadata, if the *metadata*
plugin is also enabled:

* `forward/upstream`: the upstream used to forward the request

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metric are exported:

* `coredns_forward_healthcheck_broken_total{}` - count of when all upstreams are unhealthy,
  and we are randomly (this always uses the `random` policy) spraying to an upstream.
* `coredns_forward_max_concurrent_rejects_total{}` - count of queries rejected because the
  number of concurrent queries were at maximum.
* `coredns_proxy_request_duration_seconds{proxy_name="forward", to, rcode}` - histogram per upstream, RCODE
* `coredns_proxy_healthcheck_failures_total{proxy_name="forward", to, rcode}`- count of failed health checks per upstream.
* `coredns_proxy_conn_cache_hits_total{proxy_name="forward", to, proto}`- count of connection cache hits per upstream and protocol.
* `coredns_proxy_conn_cache_misses_total{proxy_name="forward", to, proto}` - count of connection cache misses per upstream and protocol.

Where `to` is one of the upstream servers (**TO** from the config), `rcode` is the returned RCODE
from the upstream, `proto` is the transport protocol like `udp`, `tcp`, `tcp-tls`.

The following metrics have recently been deprecated:
* `coredns_forward_healthcheck_failures_total{to, rcode}`
  * Can be replaced with `coredns_proxy_healthcheck_failures_total{proxy_name="forward", to, rcode}`
* `coredns_forward_requests_total{to}`
  * Can be replaced with `sum(coredns_proxy_request_duration_seconds_count{proxy_name="forward", to})`
* `coredns_forward_responses_total{to, rcode}`
  * Can be replaced with `coredns_proxy_request_duration_seconds_count{proxy_name="forward", to, rcode}`
* `coredns_forward_request_duration_seconds{to, rcode}`
  * Can be replaced with `coredns_proxy_request_duration_seconds{proxy_name="forward", to, rcode}`

## Examples

Proxy all requests within `example.org.` to a nameserver running on a different port:

~~~ corefile
example.org {
    forward . 127.0.0.1:9005
}
~~~

Send all requests within `lab.example.local.` to `10.20.0.1`, all requests within `example.local.` (and not in
`lab.example.local.`) to `10.0.0.1`, all others requests to the servers defined in `/etc/resolv.conf`, and
caches results. Note that a CoreDNS server configured with multiple _forward_ plugins in a server block will evaluate those
forward plugins in the order they are listed when serving a request.  Therefore, subdomains should be
placed before parent domains otherwise subdomain requests will be forwarded to the parent domain's upstream.
Accordingly, in this example `lab.example.local` is before `example.local`, and `example.local` is before `.`.

~~~ corefile
. {
    cache
    forward lab.example.local 10.20.0.1
    forward example.local 10.0.0.1
    forward . /etc/resolv.conf
}
~~~

The example above is almost equivalent to the following example, except that example below defines three separate plugin
chains (and thus 3 separate instances of _cache_).

~~~ corefile
lab.example.local {
    cache
    forward . 10.20.0.1
}
example.local {
    cache
    forward . 10.0.0.1
}
. {
    cache
    forward . /etc/resolv.conf
}
~~~

Load balance all requests between three resolvers, one of which has a IPv6 address.

~~~ corefile
. {
    forward . 10.0.0.10:53 10.0.0.11:1053 [2003::1]:53
}
~~~

Forward everything except requests to `example.org`

~~~ corefile
. {
    forward . 10.0.0.10:1234 {
        except example.org
    }
}
~~~

Proxy everything except `example.org` using the host's `resolv.conf`'s nameservers:

~~~ corefile
. {
    forward . /etc/resolv.conf {
        except example.org
    }
}
~~~

Proxy all requests to 9.9.9.9 using the DNS-over-TLS (DoT) protocol, and cache every answer for up to 30
seconds. Note the `tls_servername` is mandatory if you want a working setup, as 9.9.9.9 can't be
used in the TLS negotiation. Also set the health check duration to 5s to not completely swamp the
service with health checks.

~~~ corefile
. {
    forward . tls://9.9.9.9 {
       tls_servername dns.quad9.net
       health_check 5s
    }
    cache 30
}
~~~

Or configure other domain name for health check requests

~~~ corefile
. {
    forward . tls://9.9.9.9 {
       tls_servername dns.quad9.net
       health_check 5s domain example.org
    }
    cache 30
}
~~~

Or with multiple upstreams from the same provider

~~~ corefile
. {
    forward . tls://1.1.1.1 tls://1.0.0.1 {
       tls_servername cloudflare-dns.com
       health_check 5s
    }
    cache 30
}
~~~

Or when you have multiple DoT upstreams with different `tls_servername`s, you can do the following:

~~~ corefile
. {
    forward . 127.0.0.1:5301 127.0.0.1:5302
}

.:5301 {
    forward . tls://8.8.8.8 tls://8.8.4.4 {
        tls_servername dns.google
    }
}

.:5302 {
    forward . tls://1.1.1.1 tls://1.0.0.1 {
        tls_servername cloudflare-dns.com
    }
}
~~~

The following would try 1.2.3.4 first. If the response is `NXDOMAIN`, try 5.6.7.8. If the response from 5.6.7.8 is `NXDOMAIN`, try 9.0.1.2.

~~~ corefile
. {
  forward . 1.2.3.4 {
    next NXDOMAIN
  }
  forward . 5.6.7.8 {
    next NXDOMAIN
  }
  forward . 9.0.1.2 {
  }
}
~~~

## See Also

[RFC 7858](https://tools.ietf.org/html/rfc7858) for DNS over TLS.
