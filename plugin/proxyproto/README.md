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
