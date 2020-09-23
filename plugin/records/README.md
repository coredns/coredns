# records

## Name

*records* - enables serving (basic) zone data directly from the Corefile.

## Description

The *records* plugin is useful for serving zone data that is specified inline in the configuration
file. As opposed to the *hosts* plugin, this plugin supports **all** record types. Records need to
be specifed in text representation as specified in RFC 1035. If no TTL is specified in the records,
a default TTL of 3600s is assumed.

For negative responses a SOA record should be included in the response, this will only be done when
a SOA record is included in the data.

The *records* plugin uses a simple algorithm for find the correct record(s) to return. This means
some more advanced features are currently not available, such as:

* DNSSEC, if RRSIG records are added they will not be returned in the reply even if the client is
  capable of handling them. If you need signed replies use the *dnssec* plugin in conjunction with
  this one.
* Wildcards, i.e. `*.example.org`, will not be detected as a wildcard record.

If you need a more robust implementation you probably want to use the *file* plugin.

Note the *host* plugin is configured before *records* in `plugin.cfg`, which means that when both
are being specified in a server block, the *host* plugin will get preference.

This plugin can only be used once per Server Block.

## Syntax

~~~
records [ZONES...] {
    [INLINE]
}
~~~

* **ZONES** zones it should be authoritative for. If empty, the zones from the configuration block
   are used.
* **INLINE** the resource record that are to be served. These must be specified as the text
   represenation (as specifed in RFC 1035) of the record. See the examples below. Each record must
   be on a single line.

If domain name in **INLINE** are not fully qualifed each of the **ZONES** are used as the origin and
added to the names.

## Examples

Serve a MX records for example.org *and* give the MX server the name `mx1` and address 127.0.0.1.

~~~ corefile
example.org {
    records {
        @   60  IN SOA ns.icann.org. noc.dns.icann.org. 2020091001 7200 3600 1209600 3600
        @   60  IN MX 10 mx1
        mx1 60  IN A  127.0.0.1
    }
}
~~~

Create 2 zones, each will have a MX record. Note that no SOA record has been given. Also note you
need to quote the `;` in the TXT record's data to make the parser happy. (A `;` is a comment in a
RFC 1035 zone file and everything after it will be ignored, hence the need for quoting it here.)

~~~
. {
    records example.org example.net {
        mx1 IN MX 10 mx1
        dkim._domainkey.relay 3600 IN TXT "v=DKIM1\; h=sha256\; k=rsa\; s=email\; p=MIIBIj ..."
    }
}
~~~

## Bugs

DNSSEC, nor wildcards are implemented. The lookup algorithm is pretty basic. Future enhancements
could leverage the code from the *file* plugin to make more compliant with the DNS specification.

## See Also

See the *hosts*' plugin documentation if you just need to return address records. Use the *reload*
plugin to reload the contents of these inline records automatically when they are changed. The
*dnssec* plugin can be used to sign replies. See RFC 1035 and subsequent RFCs defining new record
types for the text representation that must be used in this plugin. Note RFC 3597 (Handling of
Unknown DNS Resource Record) syntax is also supported.

Use the *file* plugin for a more fully featured DNS implementation (including DNSSEC).
