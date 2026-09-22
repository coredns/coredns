# dynupdate

## Name

*dynupdate* - accepts authenticated RFC 2136 DNS UPDATE messages for an
explicit, opt-in authoritative zone.

## Description

The *dynupdate* plugin serves one writable authoritative zone through the
normal CoreDNS authoritative file implementation and may be used only once
per server block. Use separate server blocks for separate writable zones.
An RFC 1035-style zone file provides the initial data and is never modified.
Configure `database` for a
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
boundary. UPDATE requests for a different zone receive NOTAUTH; they do not
fall through to query-only backends. Ordinary queries outside the dynamic
zone still pass to the next plugin.

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

Configuration validation does not create or modify the database. A missing
database is initialized from `file` on the first query, transfer or
authenticated UPDATE after startup. This prevents a failed startup from
preserving an obsolete seed. Until that first access, the seed must remain
available; creation errors return SERVFAIL rather than acknowledging an update.
Subsequently, the database, including the SOA serial, is authoritative;
editing or removing the seed does not replace dynamic data. Corrupt, incompatible, wrong-zone, or
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

### DHCP Client Permissions

The required permissions depend on the UPDATE messages sent by the DHCP
implementation, not just the address records it creates. For example, Kea
2.0.2 D2 writes DHCID records in both the forward and reverse zones and uses
an `ANY` deletion when releasing a name. Allowing only A/AAAA or PTR lets
some steps succeed but refuses later steps. Forward and reverse updates are
separate transactions: a rejected reverse update does not undo a successful
forward update.

For a DHCP-managed `host.example.org.` at `192.0.2.10`, the forward-zone
rule can be:

~~~ text
allow update-key.example.org. host.example.org. A AAAA DHCID ANY
~~~

The corresponding rule in `2.0.192.in-addr.arpa.` can be:

~~~ text
allow update-key.example.org. 10.2.0.192.in-addr.arpa. PTR DHCID ANY
~~~

`ANY` explicitly permits deleting all RRsets at the authorized name; it
does not mean only the other types listed in the rule. Do not place unrelated
static records at those names. Use `*` for the name only when the DHCP
updater is trusted to manage the entire zone. Keep DHCID conflict resolution
enabled on the DHCP side; a TSIG key identifies the updater, not the client
that owns a lease.

### Interoperability And Sizing

With BIND `nsupdate` and Kea `kea-dhcp-ddns` installed, run:

~~~ sh
go test -race ./test -run '^TestDynUpdate' -count=3
go test ./plugin/dynupdate -run '^$' -bench '^BenchmarkUpdate$' -benchmem -count=3
~~~

The Kea test supplies synthetic lease-change notifications to a real D2
process and verifies IPv4 and IPv6 forward/reverse creation, renewal,
ownership conflicts, removal, and name reuse. It is not a DHCP address
allocation, lease-expiration, or physical-network test. Missing client
binaries skip the corresponding local tests; Linux CI installs both.
Distribution confinement may require approved paths for the Kea test
process. `COREDNS_KEA_CONFIG_DIR`, `KEA_PIDFILE_DIR`, and `KEA_LOCKFILE_DIR`
can select prepared writable directories. Linux CI uses this facility to
keep Ubuntu's AppArmor policy enabled. Do not point a test at runtime
directories used by a live Kea service.

The benchmark changes a record in 100-, 1000-, and 10000-record zones,
with and without synchronous persistence and four concurrent query workers.
`ns/op` measures one protocol-engine transaction, excluding transport and
TSIG verification, while the query metrics report concurrent query latency
and throughput. Allocations include the query workers when
enabled. Measure on the filesystem and hardware used for deployment:
updates rebuild the entire zone and queries can wait for the update and
disk commit. The record limits bound accepted data, not update latency or
peak process memory. This backend is intended for small, infrequently
updated zones, not a high-throughput DHCP service.

## See Also

See the *file*, *transfer*, and *tsig* plugins for authoritative data,
AXFR/NOTIFY, and TSIG authentication configuration.

* [RFC 2136](https://www.rfc-editor.org/rfc/rfc2136) defines DNS UPDATE.
* [RFC 1982](https://www.rfc-editor.org/rfc/rfc1982) defines DNS serial
  number arithmetic.
