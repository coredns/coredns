# proxyproto

## Name

*proxyproto* - add [PROXY protocol](https://www.haproxy.org/download/1.8/doc/proxy-protocol.txt) support.

## Description

This plugin adds support for the PROXY protocol version 1 and 2. It allows CoreDNS to receive
connections from a load balancer or proxy that uses the PROXY protocol to forward the original
client's IP address and port information.

## Syntax

~~~ txt
proxyproto {
    allow <CIDR...>
    default <use|ignore|reject|skip>
    udp_session_tracking <duration> [max_sessions]
    tlv_metadata
}
~~~

If `allow` is unspecified, PROXY protocol headers are accepted from all IP addresses.
The `default` option controls how connections from sources not listed in `allow` are handled.
If `default` is unspecified, it defaults to `ignore`.
The possible values are:

- `use`: accept and use PROXY protocol headers from these sources
- `ignore`: accept and ignore PROXY protocol headers from other sources
- `reject`: reject connections with PROXY protocol headers from other sources
- `skip`: skip PROXY protocol processing for connections from other sources, treating them as normal connections preserving the PROXY protocol headers.

The `udp_session_tracking <duration> [max_sessions]` option enables UDP session state tracking
for Cloudflare Spectrum's PROXY Protocol v2 over UDP. Spectrum sends the PPv2 header as a
standalone first datagram (with no DNS payload). Subsequent datagrams from the same client arrive
without any header. When this option is set to a positive duration, the real client address from
the header-only datagram is cached (keyed by the Spectrum-side remote address) for that duration
and automatically applied to all subsequent headerless datagrams within that window. The TTL is
refreshed on each matching packet. The optional `max_sessions` argument caps the number of
concurrent sessions in the LRU cache (default: 10240). This option has no effect for TCP
connections.

The `tlv_metadata` option exposes the PROXY Protocol v2 [TLV](https://www.haproxy.org/download/2.3/doc/proxy-protocol.txt)
(Type-Length-Value) vector from the header to downstream plugins via the
[*metadata*](../metadata/README.md) plugin. When set, and the *metadata* plugin is enabled, the
raw TLV vector is published under the label `proxyproto/tlv` as a hex-encoded string. Consumers
decode the hex and split it with `proxyproto.SplitTLVs` from
`github.com/pires/go-proxyproto` to read individual TLVs (e.g. the AWS VPC endpoint ID, SSL
client CN, or vendor-specific custom TLVs). This option is opt-in: without it, TLVs are not
captured and no extra per-connection processing occurs. It applies to all transports: `dns://`
(UDP and TCP), `tls://` (DoT), `https://` (DoH), `grpc://`, `quic://` (DoQ), and `https3://`
(DoH3).

For the QUIC-based transports (`quic://` and `https3://`), the PPv2 header arrives as a
standalone first datagram ahead of the QUIC handshake, so TLV capture additionally requires
`udp_session_tracking` to be enabled; the cached session links the subsequent headerless QUIC
datagrams to the header's source and TLVs.

## Metadata

When the `tlv_metadata` option and the *metadata* plugin are both enabled, the following metadata
label is made available:

- `proxyproto/tlv`: the raw PPv2 TLV vector, hex-encoded. Only set when the request's connection
  carried a PPv2 header with at least one TLV.

## Examples

In this configuration, we allow PROXY protocol connections from all IP addresses:

~~~ corefile
. {
    proxyproto
    forward . /etc/resolv.conf
}
~~~

In this configuration, we only allow PROXY protocol connections from the specified CIDR ranges
and ignore proxy protocol headers from other sources:

~~~ corefile
. {
    proxyproto {
        allow 192.168.1.1/32 192.168.0.1/32
    }
    forward . /etc/resolv.conf
}
~~~

In this configuration, we only allow PROXY protocol headers from the specified CIDR ranges and reject
connections without valid PROXY protocol headers from those sources:

~~~ corefile
. {
    proxyproto {
        allow 192.168.1.1/32
        default reject
    }
    forward . /etc/resolv.conf
}
~~~

In this configuration, we enable UDP session tracking for Cloudflare Spectrum's PPv2-over-UDP
with a 28-second TTL (slightly shorter than Spectrum's 30-second UDP idle timeout) and the
default session cap of 10240:

~~~ corefile
. {
    proxyproto {
        allow 192.168.1.1/32
        udp_session_tracking 28s
    }
    forward . /etc/resolv.conf
}
~~~

In this configuration, the session cap is raised to 20480:

~~~ corefile
. {
    proxyproto {
        allow 192.168.1.1/32
        udp_session_tracking 28s 20000
    }
    forward . /etc/resolv.conf
}
~~~

In this configuration, PPv2 TLVs are exposed to downstream plugins as metadata. The *metadata*
plugin must also be enabled. Here the *log* plugin records the raw TLV vector:

~~~ corefile
. {
    metadata
    proxyproto {
        allow 192.168.1.1/32
        tlv_metadata
    }
    forward . /etc/resolv.conf
    log . "{remote} {/proxyproto/tlv}"
}
~~~
