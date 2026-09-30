# dns64

## Name

*dns64* - enables DNS64 IPv6 transition mechanism.

## Description

The *dns64* plugin will when asked for a domain's AAAA records, but only finds A records,
synthesizes the AAAA records from the A records.

The synthesis is *only* performed **if the query came in via IPv6**.

This translation is for IPv6-only networks that have [NAT64](https://en.wikipedia.org/wiki/NAT64).

Synthesised responses carry [RFC 8914](https://tools.ietf.org/html/rfc8914) Extended DNS Error code 29 ("Synthesized") so EDNS-aware clients can tell that the AAAA was produced by DNS64 rather than served from upstream. Per RFC 6891 the plugin only attaches the OPT record (and therefore the EDE) when the incoming client query already carried OPT.

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
    [allow_ipv4]
    [filter_a]
}
~~~

* `prefix` specifies any local IPv6 prefix to use, instead of the well known prefix (64:ff9b::/96)
* `translate_all` runs synthesis for every AAAA query this plugin intercepts. Real AAAA records from the upstream response are **discarded** and replaced by AAAAs synthesised from the A records of the same name. Use this when clients must always reach a host through the DNS64 prefix — for example because the prefix itself is the routing target of a gateway or subnet router.
* `allow_ipv4` Allow translating queries if they come in over IPv4, default is IPv6 only translation.
* `filter_a` suppresses client-facing A responses for queries eligible for DNS64, replying with `NOERROR` and an empty answer section. This steers clients onto the synthesised AAAA path without them ever seeing the underlying A record. The response includes:
    - An SOA for the queried name's enclosing zone, placed in the authority section so downstream resolvers can negative-cache the NODATA per [RFC 2308](https://tools.ietf.org/html/rfc2308). The SOA is harvested via a single internal SOA lookup (cheaply amortised when a `cache` plugin is in the chain). If the lookup fails or returns no SOA, the response is served without one rather than failing.
    - [RFC 8914](https://tools.ietf.org/html/rfc8914) Extended DNS Error code 17 ("Filtered"), attached only when the client's query carried OPT (RFC 6891); non-EDNS clients see plain `NOERROR`/NODATA.

  The plugin's own internal A lookup during synthesis is exempt, so AAAA responses continue to be synthesised normally.

  `filter_a` only intercepts queries of type `A`. `ANY` queries are out of scope and will reach upstream unchanged — if upstream returns A records in the `ANY` response the client will see them. Combine with the [`any`](https://coredns.io/plugins/any/) plugin in the same server block (it implements [RFC 8482](https://tools.ietf.org/html/rfc8482) and responds to `ANY` with HINFO) if you need `ANY` traffic covered as well.

### How `translate_all` and `filter_a` interact

The two options are independent and can each be used on their own. They affect different query types:

* `translate_all` changes how **AAAA** queries are answered (always synthesise vs. prefer real AAAA).
* `filter_a` changes how **A** queries are answered (suppress with NODATA + EDE 17 vs. pass through).

Client-visible behaviour for the four combinations:

| `translate_all` | `filter_a` | Client sees for A | Client sees for AAAA |
|:---:|:---:|---|---|
| off | off | real A record | real AAAA if present, otherwise synthesised from A |
| off | **on**  | NODATA + EDE 17 "Filtered" | real AAAA if present, otherwise synthesised from A |
| **on**  | off | real A record | always synthesised (real AAAA discarded) |
| **on**  | **on**  | NODATA + EDE 17 "Filtered" | always synthesised (real AAAA discarded) |

Pick based on what you want to enforce:

* Want clients to **prefer** IPv6 but still use a host's real IPv6 when it exists, and never fall back to IPv4? Use `filter_a` without `translate_all`.
* Want clients to always route through your DNS64 prefix regardless of what's upstream? Use `translate_all`, and add `filter_a` if you also want to block the v4 fallback.
* Want only synthesis, unchanged client-visible A records? Leave `filter_a` off.

**Edge case with `translate_all`**: because `translate_all` discards the upstream AAAA and rebuilds the answer from the A lookup alone, a name that has only a real AAAA and no A will yield an empty synthesised answer. Adding `filter_a` compounds this: that name becomes unresolvable for the client on both A and AAAA. In DNS64/NAT64 deployments this is usually fine (v4-only hosts are the targets), but in mixed deployments you may want to scope the dns64 block to zones where every name has A records.

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

Apply translation even to the requests which arrived over IPv4 network. Warning, the `allow_ipv4` feature will apply
translations to requests coming from dual-stack clients. This means that a request for a client that sends an `AAAA`
that would normal result in an `NXDOMAIN` would get a translated result.
This may cause unwanted IPv6 dns64 traffic when a dualstack client would normally use the result of an `A` record request.

~~~ corefile
. {
    dns64 {
        allow_ipv4
    }
}
~~~

Prefer IPv6 everywhere: suppress A responses so dual-stack clients don't fall back to v4, but keep serving real AAAA records when the upstream has them (only synthesise when there is no AAAA).

~~~ corefile
. {
    dns64 {
        filter_a
    }
    forward . 1.1.1.1
}
~~~

Force every eligible AAAA through the DNS64 prefix and block the v4 fallback. Useful when the prefix is the routing target (for example a NAT64 gateway or any transition mechanism that steers IPv6 traffic bearing the prefix to a specific router). Adding the `any` plugin short-circuits `ANY` queries with RFC 8482 HINFO, keeping v4 closed on that path too.

~~~ corefile
. {
    dns64 {
        translate_all
        filter_a
    }
    any
    forward . 1.1.1.1
}
~~~

## Metrics

If monitoring is enabled (via the _prometheus_ plugin) then the following metrics are exported:

- `coredns_dns64_requests_translated_total{server}` - counter of DNS requests translated
- `coredns_dns64_requests_filtered_total{server}` - counter of client A queries suppressed by `filter_a`

The `server` label is explained in the _prometheus_ plugin documentation.

## Bugs

Not all features required by DNS64 are implemented, only basic AAAA synthesis.

* Support "mapping of separate IPv4 ranges to separate IPv6 prefixes"
* Resolve PTR records
* Make resolver DNSSEC aware. See: [RFC 6147 Section 3](https://tools.ietf.org/html/rfc6147#section-3)

## See Also

See [RFC 6147](https://tools.ietf.org/html/rfc6147) for more information on the DNS64 mechanism.
