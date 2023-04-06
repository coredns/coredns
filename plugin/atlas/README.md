# Atlas

Atlas is a SQL DB agnostic dns provider to store domain and record resources in a database.

It uses entgo.io as orm.

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
