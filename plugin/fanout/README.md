# fanout

## Name

*fanout* - parallel proxying DNS messages to upstream resolvers.

## Description

Each incoming DNS query that hits the CoreDNS fanout plugin will be replicated in parallel to each listed IP (i.e. the DNS servers). The first non-negative response from any of the queried DNS Servers will be forwarded as a response to the application's DNS request.

## Syntax

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
  (Cloudflare) will not work.

* `fail-count` is the number of subsequent failed health checks that are needed before client stops trying to connect.
* `worker-count` is the number of parallel queries per request.
* `policy` is specifies the policy to use for selecting response from proxies.
  * `any` is a policy that returns any first response from success connected proxies.
  * `first-positive` is a policy that returns any first non-negative response from proxies.
* `network` is a specific network protocol. Could be `tcp`, `udp`, `tcp-tls`.
* `except` is a list is a space-separated list of domains to exclude from proxying.

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metric are exported:

* `coredns_fanout_request_duration_seconds{to}` - duration per upstream interaction.
* `coredns_fanout_request_count_total{to}` - query count per upstream.
* `coredns_fanout_response_rcode_count_total{to, rcode}` - count of RCODEs per upstream.
* `coredns_fanout_healthcheck_failure_count_total{to}` - number of failed health checks per upstream.
* `coredns_fanout_healthcheck_broken_count_total{}` - counter of when all upstreams are unhealthy,
  and we are randomly (this always uses the `random` policy) spraying to an upstream.

Where `to` is one of the upstream servers (**TO** from the config), `rcode` is the returned RCODE
from the upstream.

## Examples
Proxy all requests within `example.org.` to a nameservers running on a different ports.  The first positive response from a proxy will be provided as the result.

~~~ corefile
example.org {
    fanout . 127.0.0.1:9005 127.0.0.1:9006 127.0.0.1:9007 127.0.0.1:9008
}
~~~

Sends parallel requests between three resolvers, one of which has a IPv6 address via TCP. The first response from proxy will be provided as the result.

~~~ corefile
. {
    fanout . 10.0.0.10:53 10.0.0.11:1053 [2003::1]:53 {
        protocol TCP
        policy any
    }
}
~~~

Proxying everything except requests to `example.org`

~~~ corefile
. {
    fanout . 10.0.0.10:1234 {
        except example.org
    }
}
~~~

Proxy everything except `example.org` using the host's `resolv.conf`'s nameservers:

~~~ corefile
. {
    fanout . /etc/resolv.conf {
        except example.org
    }
}
~~~

Proxy all requests to 9.9.9.9 using the DNS-over-TLS protocol.
Note the `tls_servername` is mandatory if you want a working setup, as 9.9.9.9 can't be
used in the TLS negotiation.

~~~ corefile
. {
    fanout . tls://9.9.9.9 {
       tls_servername dns.quad9.net
    }
}
~~~

Sends parallel requests between five resolvers via UDP uses two workers and without attempting to reconnect. The first positive response from a proxy will be provided as the result.
~~~ corefile
. {
    fanout . 10.0.0.10:53 10.0.0.11:53 10.0.0.12:53 10.0.0.13:1053 10.0.0.14:1053 {
        worker-count 2
        max-fail-count 0
    }
}
~~~
