# siit

## Name

*siit* - enables SIIT IPv6<>IPv4 transition mechanism.

## Description

The *siit* plugin will when asked for a domain's A or AAAA records, belonging to a certain IP range,
synthesizes the corresponding AAAA or A records.

It also supports arbitrary mapping IPv4<>IPv6.

## Syntax

~~~
siit {
    ipv6_prefix IPV6PREFIX
    eam IPV4 IPV6
}
~~~

* `ipv6_prefix` specifies any local IPv6 prefix to use, instead of the well known prefix (64:ff9b::/96)
* `eam` translates the ipv4 to the corresponding ipv6 and vice-versa, it can be set multiple times

## Examples

~~~ corefile
. {
    siit {
        ipv6_prefix 64:1337::/96
    }
}
~~~

## Metrics

If monitoring is enabled (via the _prometheus_ plugin) then the following metrics are exported:

- `coredns_siit_requests_translated_total{server}` - counter of DNS requests translated

The `server` label is explained in the _prometheus_ plugin documentation.

## See Also

See [RFC 6145](https://tools.ietf.org/html/rfc6145) for more information on the SIIT mechanism
and [RFC 7757](https://tools.ietf.org/html/rfc7757) about the explicit address mappings (eam) mechanism
