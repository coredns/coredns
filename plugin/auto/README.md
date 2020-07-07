# auto

## Name

*auto* - enables serving zone data from an RFC 1035-style master file, which is automatically picked up from disk.

## Description

The *auto* plugin is used for an "old-style" DNS server. It serves from a preloaded file that exists
on disk. If the zone file contains signatures (i.e. is signed, i.e. using DNSSEC) correct DNSSEC answers
are returned. Only NSEC is supported! If you use this setup *you* are responsible for re-signing the
zonefile. New or changed zones are automatically picked up from disk only when SOA's serial changes. If the zones are not updated via a zone transfer, the serial must be manually changed.

## Syntax

~~~
auto [ZONES...] {
    directory DIR [REGEXP ORIGIN_TEMPLATE]
    reload DURATION
}
~~~

**ZONES** zones it should be authoritative for. If empty, the zones from the configuration block
are used.

* `directory` loads zones from the specified **DIR**. If a file name matches **REGEXP** it will be
  used to extract the origin. **ORIGIN_TEMPLATE** will be used as a template for the origin. Strings
  like `{<number>}` are replaced with the respective matches in the file name, e.g. `{1}` is the
  first match, `{2}` is the second. The default is: `db\.(.*)  {1}` i.e. from a file with the
  name `db.example.com`, the extracted origin will be `example.com`.
* `reload` interval to perform reloads of zones if SOA version changes and zonefiles. It specifies how often CoreDNS should scan the directory to watch for file removal and addition. Default is one minute.
  Value of `0` means to not scan for changes and reload. eg. `30s` checks zonefile every 30 seconds
  and reloads zone when serial changes.

For enabling zone transfers look at the *transfer* plugin.

All directives from the *file* plugin are supported. Note that *auto* will load all zones found,
even though the directive might only receive queries for a specific zone. I.e:

~~~ corefile
. {
    auto example.org {
        directory /etc/coredns/zones
    }
}
~~~
Will happily pick up a zone for `example.COM`, except it will never be queried, because the *auto*
directive only is authoritative for `example.ORG`.

## Examples

Load `org` domains from `/etc/coredns/zones/org` and allow transfers to the internet, but send
notifies to 10.240.1.1

~~~ corefile
org {
    auto {
        directory /etc/coredns/zones/org
    }
    transfer {
        to *
        to 10.240.1.1
    }
}
~~~

Load `org` domains from `/etc/coredns/zones/org` and looks for file names as `www.db.example.org`,
where `example.org` is the origin. Scan every 45 seconds.

~~~ corefile
org {
    auto {
        directory /etc/coredns/zones/org www\.db\.(.*) {1}
        reload 45s
    }
}
~~~

## Also

Use the *root* plugin to help you specify the location of the zone files. See the *transfer* plugin
to enable outgoing zone transfers.
