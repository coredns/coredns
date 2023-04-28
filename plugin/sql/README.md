# SQL Plugin - WIP (work in progress)

Plugin that stores Zone and Record Resource Records (RRs) in a SQL Database.

## Features

- works as authoritative server
- supports many relational databases
- supports many resource record types (35 RRs are supported)

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

## Setup

Put this into your `Corefile`.

```config
sql {
    dsn connectionstring
    [automigrate bool]
    [debug bool]
    [zone_update_duration duration]
}
```

- `dsn` is a string with the data source to which a connection is to be made
- If you set the `automigrate` option to true, the SQL plugin will migrate the database automatically. Good for development, but not recommended for production use! If the parameter is omitted, automigrate will be false by default.
- `debug` true - logs all sql statements
- `zone_update_duration` reload the zones every `N` times; default: 1 minutes, duration can be ex `30s` seconds, `1m` minute or whatever duration you want

### Prepare the Database

There are `SQL` files to create tables in the [migrations](migrations) directory for MySQL, MariaDB and PostgreSQL.

If you use the `automigrate` feature, the database will be migrated automatically.

It is not advisable to use the Automigrate function in production environments.

### Import Zone and RRs

You will find methods to [import a zone](utils/importzone.go) file into the database.

Maybe later we will provide a import executable.
