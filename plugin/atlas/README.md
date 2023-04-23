# Atlas - WIP (work in progress)

Atlas is a coredns SQL database plugin that stores zone and record resources in a relational database.

It uses [entgo.io](https://entgo.io/docs/getting-started) as ORM and [Ariga](https://ariga.io/) [Atlas](https://atlasgo.io/getting-started) for database migrations.

## Features

- works as authoritative server
- supports many relational databases
- supports many resource record types (RRs)
- supports database migrations
- TODO: supports AXFR zone transfer

## Why [Ariga Atlas](https://atlasgo.io/getting-started)?

- works with [entgo.io](https://entgo.io/docs/getting-started) (the ORM that this plugin is using)
- cool features for database schema migrations
- [CI migration support](https://atlasgo.io/integrations/github-actions) (Github Action)
- [Terraform provider](https://atlasgo.io/integrations/terraform-provider)

We don't force anyone to use Ariga Atlas! Use Flyway or whatever migration tool fits your workflow. But please don't ask for support for other tools.

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
- `zone_update_duration` reload the zones every `N` times; default: 1 minutes, duration can be ex `30s` seconds, `1m` minute or whatever duration you want

### Prepare the Database

You can use our [schema files](migrations) or Ariga Atlas (see below).

## Supported Databases

Databases, that are supported by entgo.

| Database    | Version                       | Remarks                                                                                             |
| ----------- | ----------------------------- | --------------------------------------------------------------------------------------------------- |
| SQLite3     | 3.40.x, 3.41.x                | others unknown                                                                                      |
| PostgreSQL  | 10, 11, 12, 13, 14            |                                                                                                     |
| MariaDB     | 10.2, 10.3 and latest version |                                                                                                     |
| MySQL       | 5.6.35, 5.7.26, 8             |                                                                                                     |
| CockroachDB | v21.2.11                      | Preview                                                                                             |
| TiDB        | 5.4.0, 6.0.0                  | Preview, MySQL compatible, [known issues](https://docs.pingcap.com/tidb/stable/mysql-compatibility) |
| Gremlin     | experimental                  | does not support migration nor indexes                                                              |

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

You will find the schema for your database in the Atlas [migrations](migrations) directory.

### DB Schema inspection

If you want to inspect your **existing** schema, you can use the following cli command.

#### HCL output

```shell
# postgres
atlas schema inspect -u "postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable"

# mysql
atlas schema inspect -u "mysql://${DB_USER}:${DB_PASS}@localhost:3306/${DB_NAME}"

# mariadb - please note: port 3307 is not the standard port (see docker-compose.yaml)!!!
atlas schema inspect -u "mariadb://${DB_USER}:${DB_PASS}@localhost:3307/${DB_NAME}"
```

#### SQL Output

```shell
# postgres
atlas schema inspect -u "postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable" --format '{{ sql . }}'

# mysql
atlas schema inspect -u "mysql://${DB_USER}:${DB_PASS}@localhost:3306/${DB_NAME}" --format '{{ sql . }}'

# mariadb
atlas schema inspect -u "maria://${DB_USER}:${DB_PASS}@localhost:3307/${DB_NAME}" --format '{{ sql . }}'
```

### DB Schema Apply

> **_NOTE:_** Ariga Atlas differentiates between MySQL and MariaDB schema migrations. Please use `mariadb` or `mysql` for migrations in your dsn. The coredns Atlas plugin doesnt needs this differntiation and works with `mysql` or `maria`, but it makes no difference because we are using the same database driver!

#### Migrate the Database

Please set the correct environment variables (see our [.env.sample](.env.sample)).

```shell
# postgres
atlas schema apply -u "postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable" --to file://migrations/postgres --auto-approve

# mysql
atlas schema apply -u "mysql://${DB_USER}:${DB_PASS}@localhost:3306/${DB_NAME}" --to file://migrations/mysql --auto-approve

# mariadb
atlas schema apply -u "mariadb://${DB_USER}:${DB_PASS}@localhost:3307/${DB_NAME}" --to file://migrations/mariadb --auto-approve
```

## Zone file import

There is a example cobra command [zoneImport](cli/cmd/zoneImport.go) file. You can use it to import a zone file into a database (all databases are supported).

Please have a look at [root.go](cli/cmd/root.go). You have to import `_ "github.com/coredns/coredns/plugin/atlas/ent/runtime"` to omit circle dependency import errors.

## Resource Records

![Active DNS Record Types](https://upload.wikimedia.org/wikipedia/commons/5/59/All_active_dns_record_types.png)
**The image was borrowed from Wikipedia and is hopefully correct.**

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

## Development

### Direnv

Atlas was developed on Ubuntu Linux using [direnv](https://installati.one/install-direnv-ubuntu-22-04/).

Direnv helps you to automatically loading `.env` files. We are providing a `.env.sample` that works well
with the `docker-compose.yaml`. Please copy the `.env.sample` to `.env`. If you enter the `plugin/atlas` directory the `.env` will be loaded.

### Makefile

Before you run commands, run `make docker` first.

```shell
$make                                                                                                                                                                  
docker               run docker compose
generate             run go generate
install              install Ariga Atlas
ma-apply             apply changes to maria db with Ariga Atlas
ma-import            run mariadb zoneimport from tests/pri.miek.nl for domain miek.nl 
ma-inspect           inspect maria db with Ariga Atlas
ma-status            get Atlas migration status for maria db
my-import            run mysql zoneimport from tests/pri.miek.nl for domain miek.nl 
my-inspect           inspect mysql db with Ariga Atlas
my-status            get Atlas migration status for mysql db
pg-import            run postgres zoneimport from tests/pri.miek.nl for domain miek.nl 
pg-inspect           inspect postgres db with Ariga Atlas
pg-status            get Atlas migration status for postgres db
test                 run unit tests
```

### Code Generation

Atlas is using code generation to generate most of the plugin code.

Since not all databases has native json support, we are marshal/unmarshal the RR types into the `rrdata` column in the `DnsRR` schema.

If you dont know `ent`, first read the docs to understand how `ent` is working.

### Database Schema

You will find the `schema` in this [directory](ent/schema). This is the place, where the schema will be declared. If you make changes run `go generate ./...` in the plugin directory and run `make test`.

If you break anything, please fix it, provide or change the tests and make your contribution.

## Finally

Have fun!
