# dynupdate

## Name

*dynupdate* - accepts authenticated RFC 2136 DNS UPDATE messages for an
explicit, opt-in authoritative zone.

## Description

The *dynupdate* plugin serves one writable authoritative zone through the
normal CoreDNS authoritative file implementation. An RFC 1035-style zone file
provides the initial data and is never modified. Configure `database` for a
persistent primary: a successful UPDATE is committed to the local database
before its new snapshot becomes visible or the success response is sent.
Without `database`, updates are in memory only and are lost on restart or
Corefile reload; this mode is intended for temporary data and testing.

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
the plugin cannot regenerate them after an update. IXFR and automatic DNSSEC
re-signing are not supported. The plugin is experimental; it does not provide
multi-primary replication or atomic transactions across zones. Do not expose
the UPDATE service without network controls in addition to TSIG authentication.

The *cache* plugin automatically bypasses dynamic zones, so their authoritative
queries always read the current snapshot, including after negative or positive
answers. Other middleware, such as *header*, still processes those requests and
responses, and unrelated zones remain cacheable. External recursive
caches can still retain old answers until their TTL expires. AXFR requests
pass through *transfer* and its access controls. Successful changes trigger
best-effort NOTIFY; bursts are coalesced to one in-flight notification per zone
instance.

### Persistence

`database` uses an embedded [bbolt](https://github.com/etcd-io/bbolt) database;
no etcd server or container is required. Its parent directory must exist and
be writable by CoreDNS. Use a local filesystem with working file locks and
sync semantics, not a shared network filesystem. The database is private to
one zone and one CoreDNS process. Overlapping instances in that process share
transactions and snapshots during a Corefile reload, so prerequisites cannot
race and an old instance cannot overwrite a newer generation.

On the first startup, the database is initialized from `file`. Subsequently,
the database, including the SOA serial, is authoritative; editing or removing
the seed does not replace dynamic data. Corrupt, incompatible, wrong-zone, or
over-limit databases cause an error, not a fallback to the seed. A failed
commit returns SERVFAIL without publishing the candidate snapshot or serial.
After an abrupt process exit, the database reopens at a committed transaction.

Stop CoreDNS before copying the database for an offline backup or restoring
it. Never edit, replace, or delete a live database. To deliberately reset the
zone, stop CoreDNS, back up and remove the database, then restart with the
desired seed. If initial creation fails, remove the uninitialized database
before retrying. Do not lower limits below the existing zone's size when
reloading. Database files can retain reusable free pages after records are
deleted; `max_bytes` limits live uncompressed DNS data, not on-disk file size.

## Syntax

~~~
dynupdate [ZONE] {
    file DBFILE
    database PATH
    allow KEY NAME TYPE [TYPE...]
    max_records COUNT
    max_bytes BYTES
    max_update_records COUNT
}
~~~

* **ZONE** is the single authoritative zone. If omitted, the server block
  must define exactly one zone.
* **DBFILE** is the RFC 1035-style seed zone file. A relative path is resolved
  below the path configured by the *root* plugin. Required, but read only when
  initializing a new database or starting in memory-only mode.
* `database` is optional. **PATH** is a local database file, also resolved
  relative to *root*. The database is created with mode 0600 on systems that
  support Unix file permissions.
* **KEY** is the normalized TSIG key name configured in the *tsig* plugin.
* **NAME** is an exact owner name, `@` for the zone apex, or `*` for all names
  in the zone.
* **TYPE** is one or more RR types, `ANY` to authorize deleting all RRsets at
  one owner name, or `*` for all supported update operations. A wildcard type
  must be the only type in the rule.
* `max_records` defaults to 10000 records in the zone.
* `max_bytes` defaults to 8388608 bytes of uncompressed DNS record data.
* `max_update_records` defaults to 1024 records total in an UPDATE's
  Prerequisite and Update sections.

Limits must be positive integers. Requests exceeding the configured limits
are refused atomically with REFUSED; seed or stored data above the zone limits
is rejected during startup. Updates are serialized and rebuild the bounded
zone snapshot, so this backend is intended for small dynamic zones, not
high-volume bulk loading.

At least one `allow` rule is required. The plugin owns the configured zone;
do not configure a second authoritative backend for the same zone unless its
independent behavior is explicitly intended.

## Examples

For temporary ACME challenge records, load a seed zone and permit one key to
update TXT records at the challenge owner. This example uses memory-only mode.
Generate a private key for your deployment; the example secret is public.

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

For a persistent zone, add `database`. A client that needs several record
types at selected names can use separate narrow rules:

~~~
example.org {
    tsig {
        secret update-key.example.org. i9M+00yrECfVZG2qCjr4mPpaGim/Bq+IWMiNrLjUO4Y=
        require_opcode UPDATE
    }
    dynupdate {
        file example.org.zone
        database example.org.db
        allow update-key.example.org. host.example.org. A AAAA
        allow update-key.example.org. _acme-challenge.example.org. TXT
    }
}
~~~

A minimal seed file is:

~~~ zone
$ORIGIN example.org.
@    60 IN SOA ns.example.org. hostmaster.example.org. 1 3600 600 86400 60
@    60 IN NS ns.example.org.
ns   60 IN A 192.0.2.53
~~~

With a BIND-format TSIG key file, `nsupdate -k update.key` can submit:

~~~ text
server 127.0.0.1 53
zone example.org.
prereq nxrrset host.example.org. A
update add host.example.org. 60 A 192.0.2.10
send
~~~

Use `nsupdate -v -k update.key` for TCP. Query `host.example.org. A` directly
on this server to see the change. With `database` configured it remains after
restart. For DHCP forward and reverse updates, configure each zone in its own
server block with its own seed, database and least-privilege `allow` rules.
The DHCP server remains responsible for lease expiry, record cleanup, and
coordinating its forward and reverse requests.

## See Also

See the *file*, *transfer*, and *tsig* plugins for authoritative data,
AXFR/NOTIFY, and TSIG authentication configuration.

* [RFC 2136](https://www.rfc-editor.org/rfc/rfc2136) defines DNS UPDATE.
* [RFC 1982](https://www.rfc-editor.org/rfc/rfc1982) defines DNS serial
  number arithmetic.
