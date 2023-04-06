# Atlas

Atlas is a SQL DB agnostic dns provider to store domain and record resources in a database.

It uses entgo.io as orm and [Ariga](https://ariga.io/) Atlas. Therefore the name was borrowed from that product and this software development is not related to Ariga!

Since DNS makes the world go around, we have found: Atlas is the right service name!

Moreover we are planning a GraphQL Service (closed source for now) that will work with the same database scheme, so we can better handle our day to day requirements.

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

## Supported Resource Records

| Implemented | RR         | Remark                                  |
| ----------- | ---------- | --------------------------------------- |
|             | A          | IPv4 address                            |
|             | AAAA       | IPv6 address                            |
|             | CAA        | Certification Authority Authorization   |
|             | CERT       | Certificate                             |
|             | CNAME      | Canonical Name                          |
|             | DNAME      | Delegation Name                         |
|             | DNSKEY     | DNS Key                                 |
|             | DS         | Delegation Signer                       |
|             | HINFO      | Host Information                        |
|             | IPSECKEY   | IPsec Key                               |
|             | MX         | Mail Exchange                           |
|             | NAPTR      | Naming Authority Pointer                |
|             | NS         | Name Server                             |
|             | NSEC       | Next-Secure                             |
|             | NSEC3      | Next-Secure 3                           |
|             | NSEC3PARAM | Next-Secure 3 Parameters                |
|             | OPENPGPKEY | OpenPGP Key                             |
|             | PTR        | Pointer                                 |
|             | RRSIG      | Resource Record Signature               |
|             | SOA        | Start of Authority                      |
|             | SPF        | Sender Policy Framework                 |
|             | SRV        | Service Locator                         |
|             | SSHFP      | SSH Fingerprint                         |
|             | TLSA       | Transport Layer Security Authentication |
|             | TXT        | Text                                    |

What about name flattening "ANAME" records?

## Atlas Configuration

### SQLite3

#### SQLite3 InMemory (for testing)

You should not working with in memory files, because all changes are lost after restart. This is mainly for testing purposes.

```config
atlas {
    dsn "sqlite3://file:ent?mode=memory&cache=shared&_fk=1"
}
```

### PostgreSQL

```config
atlas {
    dsn "postgres://postgres:postgres@localhost:5432/corednsdb"
}
```

### MySQL / MariaDB

```config
atlas {
    dsn "mysql://someuser:somepassword@localhost:3306/corednsdb?parseTime=True"
}
```
