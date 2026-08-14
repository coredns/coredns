# siit

## Name

*siit* - enables AAAA->A translation support for DNS records based on SIIT (IPv6->IPv4 translation).

## Description

The *siit* plugin will when asked for a domain's A record,
synthesizes it from a corresponding AAAA record if it belongs to a certain IP range.

It also supports arbitrary mapping IPv6->IPv4.

It is useful when published services are IPv6-only and emit their AAAA record accordingly
but IPv4 clients reach them through a siit routing. This plugin generates the associated A records
for these clients automatically.

## Syntax

~~~
siit {
    ipv6_prefix IPV6PREFIX
    eam IPV4 IPV6
}
~~~

* `ipv6_prefix` specifies any local IPv6 prefix to use, instead of the well known prefix (64:ff9b::/96)
* `eam` translates the ipv6 to the corresponding ipv4, it can be set multiple times

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

## Bugs

* Prefix matching in eam is not implemented yet.
* DNSSEC support is not implemented yet. The problem is the same as DNS64. See: [RFC 6147 Section 3](https://tools.ietf.org/html/rfc6147#section-3)

## See Also

See [RFC 6052](https://tools.ietf.org/html/rfc6052) for more information on the SIIT mechanism
and [RFC 7757](https://tools.ietf.org/html/rfc7757) about the explicit address mappings (eam) mechanism

## Notes

This plugin is heavily based on [dns64 plugin](../dns64).
