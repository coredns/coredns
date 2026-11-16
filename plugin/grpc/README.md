# grpc

## Name

*grpc* - facilitates proxying DNS messages to upstream resolvers via gRPC protocol.

## Description

The *grpc* plugin supports gRPC and TLS.

This plugin can only be used once per Server Block.

## Syntax

In its most basic form:

~~~
grpc FROM TO...
~~~

* **FROM** is the base domain to match for the request to be proxied.
* **TO...** are the destination endpoints to proxy to. Each endpoint must serve CoreDNS's
  DNS-over-gRPC protocol; ordinary DNS, DNS-over-TLS and DNS-over-HTTPS endpoints cannot be used.
  The number of upstreams is limited to 15.

Multiple upstreams are randomized (see `policy`) on first use. When a proxy returns an error
the next upstream in the list is tried.

Extra knobs are available with an expanded syntax:

~~~
grpc FROM TO... {
    except IGNORED_NAMES...
    tls CERT KEY CA
    tls_servername NAME
    policy random|round_robin|sequential
    fallthrough [ZONES...]
}
~~~

* **FROM** and **TO...** as above.
* **IGNORED_NAMES** in `except` is a space-separated list of domains to exclude from proxying.
  Requests that match none of these names will be passed through.
* `tls` **CERT** **KEY** **CA** define the TLS properties for TLS connection. From 0 to 3 arguments can be
  provided with the meaning as described below

  * `tls` - no client authentication is used, and the system CAs are used to verify the server certificate
  * `tls` **CA** - no client authentication is used, and the file CA is used to verify the server certificate
  * `tls` **CERT** **KEY** - client authentication is used with the specified cert/key pair.
    The server certificate is verified with the system CAs
  * `tls` **CERT** **KEY**  **CA** - client authentication is used with the specified cert/key pair.
    The server certificate is verified using the specified CA file

CoreDNS sets the minimum TLS version to TLS 1.2. The maximum TLS version, TLS 1.2 cipher suites, and
key exchange mechanisms use the Go `crypto/tls` defaults.

* `tls_servername` **NAME** sets the server name used to verify the upstream's certificate.
  For example, use `grpc.example.org` when connecting to an IP address whose certificate is issued
  for that name. Multiple upstreams must use certificates valid for the same `tls_servername`.
* `policy` specifies the policy to use for selecting upstream servers. The default is `random`.
* `fallthrough` **[ZONES...]** If a query results in NXDOMAIN from the gRPC backend, pass the request
  to the next plugin instead of returning the NXDOMAIN response. This is useful when the gRPC backend
  is authoritative for a zone but should not return authoritative NXDOMAIN responses for queries that
  don't actually belong to that zone (e.g., search path queries). If **[ZONES...]** is omitted, then
  fallthrough happens for all zones. If specific zones are listed, then only queries for those zones
  will be subject to fallthrough.

Also note the TLS config is "global" for the whole grpc proxy if you need a different
`tls-name` for different upstreams you're out of luck.

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metric are exported:

* `coredns_grpc_request_duration_seconds{to}` - duration per upstream interaction.
* `coredns_grpc_requests_total{to}` - query count per upstream.
* `coredns_grpc_responses_total{to, rcode}` - count of RCODEs per upstream.
  and we are randomly (this always uses the `random` policy) spraying to an upstream.

## Examples

Proxy all requests within `example.org.` to a nameserver running on a different port:

~~~ corefile
example.org {
    grpc . 127.0.0.1:9005
}
~~~

Load balance all requests between three resolvers, one of which has a IPv6 address.

~~~ corefile
. {
    grpc . 10.0.0.10:53 10.0.0.11:1053 [2003::1]:53
}
~~~

Forward everything except requests to `example.org`

~~~ corefile
. {
    grpc . 10.0.0.10:1234 {
        except example.org
    }
}
~~~

Proxy everything except `example.org` using the host's `resolv.conf`'s nameservers.
These nameservers must provide DNS-over-gRPC on port 53:

~~~ corefile
. {
    grpc . /etc/resolv.conf {
        except example.org
    }
}
~~~

Proxy all requests over TLS to a CoreDNS gRPC server on port 5553, and cache every answer for up
to 30 seconds. The upstream certificate `cert.pem` must be valid for `grpc.example.org`, with its
private key in `key.pem`. The proxy trusts its issuing CA through `ca.pem`. See the *tls* plugin
for the server-side TLS options.

The upstream server answers A queries for `example.org` with `192.0.2.1`:

~~~ corefile
grpc://.:5553 {
    tls cert.pem key.pem
    hosts {
        192.0.2.1 example.org
    }
}
~~~

The proxy listens for ordinary DNS queries on port 1053:

~~~ corefile
.:1053 {
    grpc . 127.0.0.1:5553 {
        tls ca.pem
        tls_servername grpc.example.org
    }
    cache 30
}
~~~

Query the proxy with `dig @127.0.0.1 -p 1053 example.org A`. Multiple gRPC upstreams can be listed
in the same stanza if their certificates are valid for the configured `tls_servername`.

Forward requests to a local upstream listening on a Unix domain socket.

~~~ corefile
. {
    grpc . unix:///path/to/grpc.sock
}
~~~

Proxy requests for `example.org.` to a gRPC backend, but fallthrough to the next plugin for NXDOMAIN responses to handle search path queries correctly.

~~~ corefile
example.org {
    grpc . 127.0.0.1:9005 {
        fallthrough
    }
    forward . 8.8.8.8
}
~~~

## Bugs

The TLS config is global for the whole grpc proxy if you need a different `tls_servername` for
different upstreams you're out of luck.
