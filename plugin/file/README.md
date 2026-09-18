# file

## Name

*file* - enables serving zone data from an RFC 1035-style master file.

## Description

The *file* plugin is used for an "old-style" DNS server. It serves from a preloaded file that exists
on disk contained RFC 1035 styled data. If the zone file contains signatures (i.e., is signed using
DNSSEC), correct DNSSEC answers are returned. Only NSEC is supported! If you use this setup *you*
are responsible for re-signing the zonefile.

## Syntax

~~~
file DBFILE [ZONES...]
~~~

* **DBFILE** the database file to read and parse. If the path is relative, the path from the *root*
  plugin will be prepended to it.
* **ZONES** zones it should be authoritative for. If empty, the zones from the configuration block
  are used.

The SOA record's owner name must match the zone being loaded. Names without a final dot are
relative to the current origin; for example, `test` in a zone loaded as `test.` becomes
`test.test.`, not `test.`. Use `@` (when the current origin matches the zone) or the zone's
absolute name for the SOA owner. A mismatched SOA causes loading to fail; an invalid reload
leaves the last successfully loaded zone in service.

If you want to round-robin A and AAAA responses look at the *loadbalance* plugin.

~~~
file DBFILE [ZONES... ] {
    reload DURATION
    reload_by_mtime
    fallthrough [ZONES...]
}
~~~

* `reload` interval to perform a reload of the zone if the SOA version changes. Default is one minute.
  Value of `0` means to not scan for changes and reload. For example, `30s` checks the zonefile every 30 seconds
  and reloads the zone when serial changes.
* `reload_by_mtime` if set, decision to reload the zone will be based on the zone file modification time,
  instead of change in the SOA serial.
* `fallthrough` If zone matches and no record can be generated, pass request to the next plugin.
  If **[ZONES...]** is omitted, then fallthrough happens for all zones for which the plugin
  is authoritative. If specific zones are listed (for example `in-addr.arpa` and `ip6.arpa`), then only
  queries for those zones will be subject to fallthrough.

If you need outgoing zone transfers, take a look at the *transfer* plugin.

## Examples

Load the `example.org` zone from `db.example.org` and allow transfers to the internet, but send
notifies to 10.240.1.1

~~~ corefile
example.org {
    file db.example.org
    transfer {
        to * 10.240.1.1
    }
}
~~~

Where `db.example.org` would contain RRSets (<https://tools.ietf.org/html/rfc7719#section-4>) in the
(text) presentation format from RFC 1035:

~~~
$ORIGIN example.org.
@	3600 IN	SOA sns.dns.icann.org. noc.dns.icann.org. 2017042745 7200 3600 1209600 3600
	3600 IN NS a.iana-servers.net.
	3600 IN NS b.iana-servers.net.

www     IN A     127.0.0.1
        IN AAAA  ::1
~~~


Or use a single zone file for multiple zones, with relative owner names and no fixed `$ORIGIN`:

~~~ corefile
. {
    file db.shared example.org example.net
    transfer example.org example.net {
        to * 10.240.1.1
    }
}
~~~

For example, `db.shared` can contain:

~~~
@   3600 IN SOA sns.dns.icann.org. noc.dns.icann.org. 2017042745 7200 3600 1209600 3600
@   3600 IN NS a.iana-servers.net.
www 3600 IN A 127.0.0.1
~~~

Each configured zone is used as the initial origin when parsing this file, so `@` is its apex.
Signed zones require signatures for the actual owner names and must be signed separately.

Note that if you have a configuration like the following you may run into a problem of the origin
not being correctly recognized:

~~~ corefile
. {
    file db.example.org
}
~~~

We omit the origin for the file `db.example.org`, so this references the zone in the server block,
which, in this case, is the root zone. A file with an SOA for `example.org.` will be rejected
because it does not match the configured root zone, even if the file sets `$ORIGIN example.org.`.
Specify the correct zone in one of two ways:

~~~ corefile
. {
    file db.example.org example.org
}
~~~

Or

~~~ corefile
example.org {
    file db.example.org
}
~~~

## See Also

See the *loadbalance* plugin if you need simple record shuffling. And the *transfer* plugin for zone
transfers. Lastly the *root* plugin can help you specify the location of the zone files.

See [RFC 1035](https://www.rfc-editor.org/rfc/rfc1035.txt) for more info on how to structure zone
files.
