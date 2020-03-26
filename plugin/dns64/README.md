# dns64

## Name

*dns64* - enables DNS64 IPv6 transition mechanism.

## Description

From Wikipedia:

> DNS64 describes a DNS server that when asked for a domain's AAAA records, but only finds
> A records, synthesizes the AAAA records from the A records.

The synthesis in only performed if the query came in via IPv6.

This translation is for IPv6-only networks that have [NAT64](https://en.wikipedia.org/wiki/NAT64).

See [RFC 6147](https://tools.ietf.org/html/rfc6147) for more information.

## Syntax

~~~
dns64 [PREFIX] {
  [translate_all]
}
~~~

* [PREFIX] defines a custom prefix instead of the default `64:ff9b::/96`.
* `translate_all` translates all queries, including respones that have AAAA results.

## Examples

Translate with the default well known prefix. Applies to all queries.

~~~
dns64
~~~

Use a custom prefix.

~~~
dns64 64:1337::/96
# Or
dns64 {
    prefix 64:1337::/96
}
~~~

Enable translation even if an existing AAAA record is present.

~~~
dns64 {
    translate_all
}
~~~

* `prefix` specifies any local IPv6 prefix to use, instead of the well known prefix (64:ff9b::/96)

## Bugs

Not all features required by DNS64 are implemented, only basic AAAA synthesis.

* Support "mapping of separate IPv4 ranges to separate IPv6 prefixes"
* Resolve PTR records
* Follow CNAME records
* Make resolver DNSSEC aware. See: [RFC 6147 Section 3](https://tools.ietf.org/html/rfc6147#section-3)
