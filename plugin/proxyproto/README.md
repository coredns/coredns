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
