# records

## Name

*records* - enables serving zone data directly from the Corefile.

## Description

The *records* plugin is useful for serving zone data that is specified inline in the configuration
file. As opposed to the *hosts* plugin, this plugin supports **all** record types.
If no TTL is specified in the records, a default TTL of 3600s is assumed. For negative responses a
SOA record should be included in the response, this will only be done when a SOA record is included
in the data.

Currently not implemented is DNSSEC. If RRSIG records are added they will not be returned in the
reply even if the client is capable of handling them. If you need signed replies use the *dnssec*
plugin in conjunction with this one.

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
  represenation of the record, see the examples below. Each record must be on a single line.

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

Create 2 zones, each will have a MX record. Note the no SOA record has been given.

~~~
. {
    records example.org example.net {
        mx1 IN MX 10 mx1
    }
}
~~~

## Bugs

DNSSEC is not implemented.

## See Also

See the *hosts*' plugin documentation if you just need to return address records. Use the *reload*
plugin to reload the contents of these inline records automatically when they are changed. The
*dnssec* plugin can be used to sign replies.
