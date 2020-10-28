# dns64

## Name

*dns64* - enables DNS64 IPv6 transition mechanism.

## Description

The *dns64* plugin will when asked for a domain's AAAA records, but only finds A records,
synthesizes the AAAA records from the A records.

The synthesis is *only* performed **if the query came in via IPv6**.

This translation is for IPv6-only networks that have [NAT64](https://en.wikipedia.org/wiki/NAT64).

## Syntax

~~~
dns64 [PREFIX]
~~~

* **PREFIX** defines a custom prefix instead of the default `64:ff9b::/96`.

Or use this slightly longer form with more options:

~~~
dns64 [PREFIX] {
    [translate_all]
    prefix PREFIX
}
~~~

* `prefix` specifies any local IPv6 prefix to use, instead of the well known prefix (64:ff9b::/96)
* `translate_all` translates all queries, including responses that have AAAA results.

## Examples

Translate with the default well known prefix. Applies to all queries (if they came in over IPv6).

~~~
. {
    dns64
}
~~~

Use a custom prefix.

~~~ corefile
. {
    dns64 64:1337::/96
}
~~~

Or
~~~ corefile
. {
    dns64 {
        prefix 64:1337::/96
    }
}
~~~

Enable translation even if an existing AAAA record is present.

~~~ corefile
. {
    dns64 {
        translate_all
    }
}
~~~

## Metrics

If monitoring is enabled (via the _prometheus_ plugin) then the following metrics are exported:

- `coredns_dns64_requests_translated_total{server}` - counter of DNS requests translated

The `server` label is explained in the _prometheus_ plugin documentation.

## Bugs

Not all features required by DNS64 are implemented, only basic AAAA synthesis.

* Support "mapping of separate IPv4 ranges to separate IPv6 prefixes"
* Resolve PTR records
* Make resolver DNSSEC aware. See: [RFC 6147 Section 3](https://tools.ietf.org/html/rfc6147#section-3)

## See Also

See [RFC 6147](https://tools.ietf.org/html/rfc6147) for more information on the DNS64 mechanism.
