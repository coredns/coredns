# Atlas - WIP (work in progress)

Atlas is a coredns SQL database plugin that stores zone and record resources in a relational database.

It uses [entgo.io](https://entgo.io/docs/getting-started) as orm and [Ariga](https://ariga.io/) [Atlas](https://atlasgo.io/getting-started) for database migrations.

Why [Ariga Atlas](https://atlasgo.io/getting-started)?

- works with [entgo.io](https://entgo.io/docs/getting-started) (the orm that this plugin is using)
- cool features for database schema migrations
- [CI migration support](https://atlasgo.io/integrations/github-actions) (Github Action)
- [Terraform provider](https://atlasgo.io/integrations/terraform-provider)

We don't force anyone to use Atlas! Use Flyway or whatever migration tool fits your workflow. But don't ask for support for other tools!

## Setup

Put this into your `Corefile`.

```config
atlas {
    dsn connectionstring
    [automigrate bool]
    [debug bool]
    [zone_update_duration duration]
}
```

- `dsn` is a string with the data source to which a connection is to be made
- If you set the `automigrate` option to true, the Atlas plugin will migrate the database automatically. Good for development, but not recommended for production use! If the parameter is omitted, automigrate will be false by default.
- `debug` true - logs all sql statements
- `zone_update_duration` reload the zones every `N` times; default: 1 minutes, duration can be ex `30s`econds, `1m`inute

## Supported Databases

Databases, that are supported by entgo.

| Database    | Version                       | Remarks                   |
| ----------- | ----------------------------- | ------------------------- |
| SQLite3     | 3.40.x, 3.41.x                | others unknown            |
| PostgreSQL  | 10, 11, 12, 13, 14            |                           |
| MariaDB     | 10.2, 10.3 and latest version |                           |
| MySQL       | 5.6.35, 5.7.26, 8             |                           |
| CockroachDB | v21.2.11                      | Preview                   |
| TiDB        | 5.4.0, 6.0.0                  | Preview, MySQL compatible |

## Database Configuration

The credentials for the database can be read from a file or set directly in the `Corefile`.

### Example configurations

#### Automigration with infile dsn

```config
atlas {
    dsn postgres://postgres:postgres@localhost:5432/corednsdb?sslmode=disable
    automigrate true
}
```

#### Automigration with file dsn

```config
atlas {
    file /path/to/vault-agent/generated/dsnfile.json
    automigrate true
}
```

The `dsnfile.json` has following expected format:

```json
{
  "dsn": "sqlite3://file:ent?mode=memory&cache=shared&_fk=1"
}
```

### SQLite3

#### SQLite3 InMemory (for testing)

> **_NOTE:_** If you want to use SQLite3, you have to compile coredns with `CGO_ENABLED=1`!

```config
atlas {
    dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
}
```

### PostgreSQL / CockroachDB

> **_NOTE:_** Socket connections are currently not supported.

```config
atlas {
    dsn postgres://postgres:postgres@localhost:5432/corednsdb?sslmode=disable
}
```

### MySQL / MariaDB / TiDB

> **_NOTE:_** Socket connections are currently not supported.

```config
atlas {
    dsn mysql://someuser:somepassword@localhost:3306/corednsdb?parseTime=True
}
```

### Read DSN from Credentials File

> **_NOTE:_** Atlas does not detect file changes after starting coredns! Credential rotation is not supported at the moment.

The credentials can be read from a json file.

If it is a relative path, the current working directory is concatenated with the config path.

```config
atlas {
    file ./secrets/from/vault.json
}
```

The JSON config file has the following format:

```json
{
  "dsn": "postgres://postgres:secret@localhost:5432/corednsdb?sslmode=disable"
}
```

## Database Migrations

Please install Atlas as described in the [Atlas doc](https://atlasgo.io/getting-started), use Atlas on Docker or use the provided SQL migration files.

You find the schema for your database in the Atlas [migrations](migrations) directory.

### DB Schema inspection

If you want to inspect your existing schema, you can use the following cli command.

> **_NOTE:_** please go into the `plugin/atlas/migrations` directory if you want to generate migration files.

#### HCL output

```shell
atlas schema inspect -u "postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable" > schema.hcl
```

#### SQL Output

```shell
atlas schema inspect -u "postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable" --format '{{ sql . }}' > pg-schema.sql

```

### DB Schema Apply

> **_NOTE:_** Ariga Atlas differentiates between MySQL and MariaDB schema migrations. Please use `mariadb` or `mysql` for migrations in your dsn. The coredns Atlas plugin doesnt needs this differntiation and works with `mysql` only!

#### HCL file migration

If you use Atlas, you can use the `hcl` file for all supported databases. Please provide the correct DSN.

```shell
atlas schema apply -u "postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable" --to file://schema.hcl
```

If no changes are made, you'll get the message:

```shell
Schema is synced, no changes to be made.
```

#### Postgres SQL file migration

```shell
atlas schema apply -u "postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable" --to file://pg-schema.sql
```

#### MySQL SQL file migration

TODO

## Zone file import

There is a example cobra command [zoneImport](cli/cmd/zoneImport.go) file. You can use it to import a zone file into a postgres database.

Please have a look at [root.go](cli/cmd/root.go). You have to import `_ "github.com/coredns/coredns/plugin/atlas/ent/runtime"` to omit circle import errors.b

## Resource Records

![Active DNS Record Types](https://upload.wikimedia.org/wikipedia/commons/5/59/All_active_dns_record_types.png)
Image by Wikipedia and hopefully correct.

This overview shows the implemented resource record types.

> **_ABBREVIATIONS:_**
>
> `bps`: coredns backend_plugin support
>
> `mh`: must have
>
> `zi`: zone import from file into database implemented ([example import](https://github.com/jproxx/coredns/blob/feature/atlas/plugin/atlas/cli/cmd/zoneImport.go))
>
> `rt`: record type exists and marshal/unmarshalling implemented
>
> `impl.`: implemented

### DNS (Meta) RR (Resource Record) Types

| RR     | bps | mh  | zi   | rt    | Remark                                                                                                                      |
| ------ | --- | --- | ---- | ----- | --------------------------------------------------------------------------------------------------------------------------- |
| NS     | ✓   | ✓   | ✓    | ✓     | Name Server                                                                                                                 |
| CNAME  | ✓   | ✓   | ✓    | ✓     | Canonical Name                                                                                                              |
| PTR    | ✓   | ✓   | ✓    | ✓     | Pointer                                                                                                                     |
| OPT    |     |     | TODO | TODO  | EDNS Option ([miekg/dns](https://github.com/miekg/dns/blob/a6f978594be8a97447dd1a5eab6df481c7a8d9dc/edns.go#L71))           |
| SOA    | ✓   | ✓   | ✓    | impl. | Start of Authority (implemented as DnsZone)                                                                                 |
| DNAME  |     |     | ✓    | ✓     | Delegation Name                                                                                                             |
| NAPTR  |     |     | ✓    | ✓     | Naming Authority Pointer                                                                                                    |
| CSYNC  |     |     | ✓    | ✓     | Child-to-Parent Synchronization                                                                                             |
| TKEY   |     |     | ✓    | ✓     | Transaction Key                                                                                                             |
| TSIG   |     |     | TODO | TODO  | Transaction Signature ([miekg/dns](https://github.com/miekg/dns/blob/a6f978594be8a97447dd1a5eab6df481c7a8d9dc/tsig.go#L97)) |
| ZONEMD |     |     | ✓    | ✓     | Message Digest for DNS Zones                                                                                                |

### IP RR Types

| RR       | bps | mh  | zi   | rt   | Remark                       |
| -------- | --- | --- | ---- | ---- | ---------------------------- |
| A        | ✓   | ✓   | ✓    | ✓    | IPv4 address                 |
| AAAA     | ✓   | ✓   | ✓    | ✓    | IPv6 address                 |
| APL      |     |     | TODO | TODO | Adress Prefix List           |
| DHCID    |     |     | ✓    | ✓    | DHCP Identifier              |
| HIP      |     |     | ✓    | ✓    | Host Identification Protocol |
| IPSECKEY |     |     | TODO | ✓    | IPsec Key                    |

### Informational RR Types

| RR    | bps | mh  | zi  | rt  | Remark                |
| ----- | --- | --- | --- | --- | --------------------- |
| TXT   | ✓   | ✓   | ✓   | ✓   | Text                  |
| HINFO |     | ✓   | ✓   | ✓   | Host Information      |
| RP    |     |     | ✓   | ✓   | Responsible Person    |
| LOC   |     |     | ✓   | ✓   | Geographical Location |

### Service Discovery RR Types

| RR  | bps | mh  | zi  | rt  | Remark          |
| --- | --- | --- | --- | --- | --------------- |
| SRV | ✓   | ✓   | ✓   | ✓   | Service Locator |

### Email RR Types

| RR         | bps | mh  | zi  | rt  | Remark             |
| ---------- | --- | --- | --- | --- | ------------------ |
| MX         | ✓   | ✓   | ✓   | ✓   | Mail Exchange      |
| SMIMEA     |     |     | ✓   | ✓   | S/Mime Association |
| OPENPGPKEY |     |     | ✓   | ✓   | OpenPGP Key        |

### DNSEC RR Types

| RR         | bps | mh  | zi   | rt    | Remark                     |
| ---------- | --- | --- | ---- | ----- | -------------------------- |
| DNSKEY     |     |     | TODO | ✓     | DNSSEC Key                 |
| RRSIG      |     |     | TODO | ✓     | Resource Record Signature  |
| NSEC3      |     |     | TODO | ✓     | Next-Secure 3              |
| DS         |     |     | ✓    | ✓     | Delegation Signer          |
| TA         |     |     | TODO | ✓     | DNSSEC Trust Authorities   |
| CDNSKEY    |     |     | TODO | TODO! | Child Copy of DNSKEY       |
| NSEC       |     |     | TODO | ✓     | Next Secure                |
| NSEC3PARAM |     |     | TODO | ✓     | Next-Secure 3 Parameters   |
| CDS        |     |     | TODO | TODO! | Child Copy of DS           |
| DLV        |     |     | TODO | TODO! | DNSEC Lookaside Validation |

### Security RR Types

| RR    | bps | mh  | zi   | rt  | Remark                                  |
| ----- | --- | --- | ---- | --- | --------------------------------------- |
| SSHFP |     |     | ✓    | ✓   | SSH Public Key Fingerprint              |
| TLSA  |     |     | ✓    | ✓   | Transport Layer Security Authentication |
| CERT  |     |     | TODO | ✓   | Certificate                             |
| KX    |     |     | ✓    | ✓   | Key Exchange                            |
| CAA   |     |     | ✓    | ✓   | Certification Authority Authorization   |

### Miscellaneous RR Types

> **_NOTE:_** What about name flattening `ANAME` records?

| RR    | bps | mh  | zi   | rt   | Remark                                                                                                                 |
| ----- | --- | --- | ---- | ---- | ---------------------------------------------------------------------------------------------------------------------- |
| AFSDB |     |     | ✓    | ✓    | AFS Database Location                                                                                                  |
| EUI48 |     |     | ✓    | ✓    | MAC Address (EUI-48)                                                                                                   |
| EUI64 |     |     | ✓    | ✓    | MAC Address (EUI-64)                                                                                                   |
| URI   |     |     | ✓    | ✓    | Uniform Resource Identifier                                                                                            |
| SVCB  |     |     | TODO | TODO | Service Binding ([miegk/dns](https://github.com/miekg/dns/blob/a6f978594be8a97447dd1a5eab6df481c7a8d9dc/svcb.go#L218)) |
| HTTPS |     |     | TODO | TODO | HTTPS Binding ([miegk/dns](https://github.com/miekg/dns/blob/a6f978594be8a97447dd1a5eab6df481c7a8d9dc/svcb.go#L231))   |
