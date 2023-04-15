# Atlas

Atlas is a SQL DB agnostic dns provider to store domain and record resources in a database.

It uses entgo.io as orm and [Ariga](https://ariga.io/) Atlas. Therefore the name was borrowed from that product and this software development is not related to Ariga!

Since DNS makes the world go around, we have found: Atlas is the right service name!

Moreover we are planning a GraphQL Service (closed source for now) that will work with the same database scheme, so we can better handle our day to day requirements.

## Supported Resource Records

TODO: Check which RR's should/must be implemented.

| backend_plugin | Must Have | Implemented | RR         | Remark                                  |
| -------------- | --------- | ----------- | ---------- | --------------------------------------- |
| ✓              | ✓         |             | A          | IPv4 address                            |
| ✓              | ✓         |             | AAAA       | IPv6 address                            |
|                | ✓         |             | CAA        | Certification Authority Authorization   |
| ✓              | ✓         |             | CNAME      | Canonical Name                          |
| ✓              | ✓         |             | MX         | Mail Exchange                           |
| ✓              | ✓         |             | NS         | Name Server                             |
| ✓              | ✓         |             | SOA        | Start of Authority                      |
| ✓              | ✓         |             | PTR        | Pointer                                 |
|                | ✓         |             | SPF        | Sender Policy Framework                 |
| ✓              | ✓         |             | SRV        | Service Locator                         |
| ✓              | ✓         |             | TXT        | Text                                    |
|                |           |             | CERT       | Certificate                             |
|                |           |             | DNAME      | Delegation Name                         |
|                |           |             | DNSKEY     | DNS Key                                 |
|                |           |             | DS         | Delegation Signer                       |
|                |           |             | HINFO      | Host Information                        |
|                |           |             | IPSECKEY   | IPsec Key                               |
|                |           |             | NAPTR      | Naming Authority Pointer                |
|                |           |             | NSEC       | Next-Secure                             |
|                |           |             | NSEC3      | Next-Secure 3                           |
|                |           |             | NSEC3PARAM | Next-Secure 3 Parameters                |
|                |           |             | OPENPGPKEY | OpenPGP Key                             |
|                |           |             | RRSIG      | Resource Record Signature               |
|                |           |             | SSHFP      | SSH Fingerprint                         |
|                |           |             | TLSA       | Transport Layer Security Authentication |

What about name flattening "ANAME" records?

## Setup

```config
atlas {
    dsn connectionstring
    [automigrate bool]
    [debug bool]
}
```

- `dsn` is a string with the data source to which a connection is to be made
- If you set the `automigrate` option to true, the Atlas plugin will migrate the database automatically. Good for development, but not recommended for production use! If the parameter is omitted, automigrate will be false by default.
- `debug` true - logs all sql statements

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
