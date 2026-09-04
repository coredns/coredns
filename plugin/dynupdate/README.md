# dynupdate

## Name

*dynupdate* - accepts authenticated RFC 2136 DNS UPDATE messages for an
explicit, opt-in authoritative zone.

## Description

The *dynupdate* plugin loads one RFC 1035-style zone file as a read-only seed
and serves the resulting zone through the normal CoreDNS authoritative file
implementation. Successful updates are applied atomically to an in-memory
snapshot; the seed file is not modified and changes are lost when CoreDNS
restarts. This is an experimental protocol-engine stage, not yet a durable
primary-master implementation.

UPDATE requests must carry a TSIG that has been validated by the *tsig*
plugin. The *dynupdate* plugin does not receive or store TSIG secrets. Every
mutation must also match an explicit `allow` rule containing the key name,
owner name, and RR type. Use `*` as the owner name or RR type only when that
broader permission is intentional. Configure `require_opcode UPDATE` in the
*tsig* plugin so unsigned UPDATE requests are rejected at the protocol
boundary.

The implementation supports RFC 2136 prerequisites, add and delete
operations, CNAME and apex SOA/NS invariants, automatic SOA serial updates,
and the current snapshot for AXFR. SOA serial zero is rejected because RFC
2136 recommends avoiding it for interoperability, and automatic increments
skip zero after wraparound. DNSSEC records and related zone-integrity metadata
(`SIG`, `KEY`, `NXT`, `DS`, `RRSIG`, `NSEC`, `DNSKEY`, `NSEC3`, `NSEC3PARAM`,
`TALINK`, `CDS`, `CDNSKEY`, `TA`, `DLV`, and `ZONEMD`) are rejected because
this stage cannot regenerate them after an update. IXFR, automatic DNSSEC
re-signing, and durable storage are not supported yet. RFC 2136 expects a
successful update to be committed to nonvolatile storage before the response;
this stage intentionally does not provide that guarantee. Do not expose the
UPDATE service without network controls in addition to TSIG authentication.

If the *cache* plugin is enabled in the same server block, exclude the dynamic
zone so existing query answers cannot outlive an update:

~~~ corefile
cache {
    disable success example.org
    disable denial example.org
}
~~~

## Syntax

~~~
dynupdate [ZONE] {
    file DBFILE
    allow KEY NAME TYPE [TYPE...]
}
~~~

* **ZONE** is the single authoritative zone. If omitted, the server block
  must define exactly one zone.
* **DBFILE** is the RFC 1035-style seed zone file. A relative path is resolved
  below the path configured by the *root* plugin.
* **KEY** is the normalized TSIG key name configured in the *tsig* plugin.
* **NAME** is an exact owner name, `@` for the zone apex, or `*` for all names
  in the zone.
* **TYPE** is one or more RR types, `ANY` to authorize deleting all RRsets at
  one owner name, or `*` for all supported update operations. A wildcard type
  must be the only type in the rule.

At least one `allow` rule is required. The plugin owns the configured zone;
do not configure a second authoritative backend for the same zone unless its
independent behavior is explicitly intended.

## Examples

Load a seed zone and permit one key to update TXT records for an ACME
challenge owner. The *tsig* plugin is deliberately configured to require TSIG
for UPDATE messages.

~~~ corefile
example.org {
    tsig {
        secret update-key.example.org. i9M+00yrECfVZG2qCjr4mPpaGim/Bq+IWMiNrLjUO4Y=
        require_opcode UPDATE
    }
    dynupdate {
        file example.org.zone
        allow update-key.example.org. _acme-challenge.example.org. TXT
    }
}
~~~

For a client that needs to update several record types at selected names, use
separate narrow rules rather than granting all types to the whole zone:

~~~
example.org {
    tsig {
        secret update-key.example.org. i9M+00yrECfVZG2qCjr4mPpaGim/Bq+IWMiNrLjUO4Y=
        require_opcode UPDATE
    }
    dynupdate {
        file example.org.zone
        allow update-key.example.org. host.example.org. A AAAA
        allow update-key.example.org. _acme-challenge.example.org. TXT
    }
}
~~~

## See Also

See the *file*, *transfer*, and *tsig* plugins for authoritative data,
AXFR/NOTIFY, and TSIG authentication configuration.

* [RFC 2136](https://www.rfc-editor.org/rfc/rfc2136) defines DNS UPDATE.
* [RFC 1982](https://www.rfc-editor.org/rfc/rfc1982) defines DNS serial
  number arithmetic.
