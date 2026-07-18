# secondary

## Name

*secondary* - enables serving a zone retrieved from a primary server.

## Description

With *secondary* you can transfer (via AXFR) a zone from another server. The retrieved zone is
*not committed* to disk (a violation of the RFC). This means restarting CoreDNS will cause it to
retrieve all secondary zones.

If the primary server(s) don't respond when CoreDNS is starting up, the AXFR will be retried
indefinitely every 10s.

## Syntax

~~~
secondary [ZONES...]
~~~

* **ZONES** zones it should be authoritative for. If empty, the zones from the configuration block
    are used. Note that without a remote address to *get* the zone from, the above is not that useful.

A working syntax would be:

~~~
secondary [zones...] {
    transfer from ADDRESS [ADDRESS...]
    catalog [MEMBER-ZONES...]
    fallthrough [ZONES...]
}
~~~

*  `transfer from` specifies from which **ADDRESS** to fetch the zone. It can be specified multiple
   times; if one does not work, another will be tried. Transferring this zone outwards again can be
   done by enabling the *transfer* plugin.

*  `catalog` treats the transferred zone as an RFC 9432 catalog zone. After each successful catalog
   transfer, CoreDNS adds and removes the catalog member zones and transfers those member zones from
   the same primary servers. Optional **MEMBER-ZONES** restrict which member zone names are accepted;
   each name also matches its subdomains. With no **MEMBER-ZONES**, all member zones are accepted for
   backward compatibility. RFC 9432 Section 7 recommends configuring this restriction because a
   catalog producer otherwise controls which zones the consumer serves. A member in another catalog
   remains a name clash unless the current catalog's `coo` property points to the newly updated
   catalog. During that ownership migration, CoreDNS preserves the current zone data only when both
   catalogs use the same member node label.

*  `fallthrough` If a query for a record in the zone results in NXDOMAIN, the query will be passed
   to the next plugin in the chain. If **[ZONES...]** are listed, then only queries for those zones
   will be subject to fallthrough. This can be useful in split DNS setups where the secondary zone
   contains only partial records.

When a zone is due to be refreshed (refresh timer fires) a random jitter of 5 seconds is applied,
before fetching. In the case of retry this will be 2 seconds. If there are any errors during the
transfer in, the transfer fails; this will be logged.

## Examples

Transfer `example.org` from 10.0.1.1, and if that fails try 10.1.2.1.

~~~ corefile
example.org {
    secondary {
        transfer from 10.0.1.1 10.1.2.1
    }
}
~~~

Or re-export the retrieved zone to other secondaries.

~~~ corefile
example.net {
    secondary {
        transfer from 10.1.2.1
    }
    transfer {
        to *
    }
}
~~~

Restrict a catalog consumer to member zones at or below `example.org` and `internal.example`.

~~~ corefile
catalog.example {
    secondary {
        transfer from 10.1.2.1
        catalog example.org internal.example
    }
}
~~~

## Bugs

Only AXFR is supported and the retrieved zone is not committed to disk.

## See Also

See the *transfer* plugin to enable zone transfers _to_ other servers.
RFC 5936 details the AXFR protocol, and RFC 9432 defines DNS catalog zones.
