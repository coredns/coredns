# fanout

## Name

*fanout* - parallel proxying DNS messages to upstream resolvers.

## Description

TODO: add description

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

* `fail-count`
* `worker-count`
* `policy`
* `network`
* `except`

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
TODO: Add examples