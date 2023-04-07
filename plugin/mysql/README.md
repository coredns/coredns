# mysql_extend

## Name

*mysql_extend* - Use mysql as backend to store dns records. English - [中文](./README_zh.md)

## Description

The mysql_extend plugin use mysql as backend to store dns records. This plug-in does not depend heavily on the stability of mysql. 

Other features of the plug-in: 
1. It has a connection pool with mysql, which can reuse the underlying tcp connection
2. Support pan domain name query
3. Support recursive query 
4. Support online function, Only online filed not equal 0 will be effective
5. Support CNAME, A, AAAA, SOA, NS and other records query 
6. Absolutely high availability without relying on mysql, you can load DNS record data through local json files 
7. Rich monitoring indicator information
8. Rich debug logs
9. If mysql table not exist, will auto create it use `zone_tables` and `record_tables`


## Compilation

This package will always be compiled as part of CoreDNS and not in a standalone way. It will require you to use `go get` or as a dependency on [plugin.cfg](https://github.com/coredns/coredns/blob/master/plugin.cfg).

The [manual](https://coredns.io/manual/toc/#what-is-coredns) will have more information about how to configure and extend the server with external plugins.

A simple way to consume this plugin, is by adding the following on [plugin.cfg](https://github.com/coredns/coredns/blob/master/plugin.cfg), and recompile it as [detailed on coredns.io](https://coredns.io/2017/07/25/compile-time-enabling-or-disabling-plugins/#build-with-compile-time-configuration-file).

~~~
mysql:github.com/snail2sky/coredns_mysql_extend
~~~

Put this early in the plugin list, so that *mysql_extend* is executed before any of the other plugins.

After this you can compile coredns by:

``` sh
go generate
go build
```

## Syntax

~~~ txt
mysql {
    dsn username:password@tcp(127.0.0.1:3306)/dns
    # The following is the default value, if there is no custom requirement, you can leave it blank
    [dump_file dump_dns.json]
    [ttl 360]
    [zones_table   zones]
    [records_table records]
    [db_max_idle_conns 4]
    [db_max_open_conns 8]
    [db_conn_max_idle_time 1h]
    [db_conn_max_life_time 24h]
    [fail_heartbeat_time 10s]
    [success_heartbeat_time 60s]
    [query_zone_sql "SELECT id, zone_name FROM %s"]
    [query_record_sql "SELECT id, zone_id, hostname, type, data, ttl FROM  %s WHERE online!=0 and zone_id=? and hostname=? and type=?"]
}
~~~

## Configuration

- `dsn` <DSN>: Connect mysql url, detail to see https://github.com/go-sql-driver/mysql#dsn-data-source-name. Default value is `username:password@tcp(127.0.0.1:3306)/dns`
- `dump_file` <FILE_PATH_STRING>: Use this file to dump and load data, if database error, this feature will be very effective. Default value is `dump_dns.json`
- `ttl` <TTL_INT>: If query ttl value from database less equal 0, this value will be used. Default value is `360`
- `zones_table` <TABLE_NAME_STRING>: Query database to get all zones, and these zones will be cached to improve efficiency. Default value is `zones`
- `records_table` <TABLE_NAME_STRING>: Query database to get records. Default value is `records`
- `db_max_idle_conns` <INT>: Set db connection pool param. Default value is `4`
- `db_max_open_conns` <INT>: Set db connection pool param. Default value is `8`
- `db_conn_max_idle_time` <TIME_DURATION>: Set db connection pool param. Default value is `1h`
- `db_conn_max_life_time` <TIME_DURATION>: Set db connection pool param. Default value is `24h`
- `fail_heartbeat_time` <TIME_DURATION>: Re get zone or re ping DB fail interval. Default value is `10s`
- `success_heartbeat_time` <TIME_DURATION>: Re get zone or re ping DB success interval. Default value is `60s`
- `query_zone_sql` <SQL_FORMAT>: Set query database sql, if you want to optimize sql. Default value is `"SELECT id, zone_name FROM %s"`
- `query_record_sql` <SQL_FORMAT>: Set query database sql, if you want to optimize sql. Default value is `"SELECT id, zone_id, hostname, type, data, ttl FROM  %s WHERE online!=0 and zone_id=? and hostname=? and type=?"`

## Metrics

In this configuration, we use this plugin to process all domain name queries ending with internal, and use the cache plugin to improve efficiency

* `open_mysql_total{status}` - Counter of open mysql instance.
* `create_table_total{status, table_name}` - Counter of create table.
* `degrade_cache_total{option, status, fqdn, qtype}` - Counter of degrade cache.
* `zone_find_total{status}` - Counter of zone find.
* `call_next_plugin_total{fqdn, qtype}` - Counter of next plugin call.
* `query_db_total{status}` - Counter of query db.
* `make_answer_total{status}` - Counter of make answer count.
* `db_ping_total{status}` - Counter of DB ping.
* `db_get_zone_total{status}` - Counter of db get zone.

The `status` label indicated which status of this metric option.
The `table_name` label indicated which option what table.
The `option` label indicated which option of this metric operate.
The `fqdn` label indicated which dns query of fqdn.
The `qtype` label indicated which dns query of type.


## Examples

- In this configuration, we use this plugin to process all domain name queries ending with internal, and use the cache plugin to improve efficiency
- Suggestion: put the area that needs to be queried into the same mysql plugin, otherwise you need to change the value specified by dump_file to prevent data inconsistency caused by repeated writing of a file
~~~ corefile
internal.:53 in-addr.arpa.:53 {
  cache
  mysql {
    dsn db_reader:qwer123@tcp(10.0.0.1:3306)/dns
    dump_file dns.json
  }
}
~~~

~~~ sql
-- Default create table SQL are
CREATE TABLE IF NOT EXISTS  zones  (
    `id` INT NOT NULL AUTO_INCREMENT,
    `zone_name` VARCHAR(255) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY (zone_name)
);

CREATE TABLE IF NOT EXISTS records (
    `id` INT NOT NULL AUTO_INCREMENT,
    `zone_id` INT NOT NULL,
    `hostname` VARCHAR(512) NOT NULL,
    `type` VARCHAR(10) NOT NULL,
    `data` VARCHAR(1024) NOT NULL,
    `ttl` INT NOT NULL DEFAULT 120,
    `online` INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    FOREIGN KEY (zone_id) REFERENCES ` + m.zonesTable + `(id)
)

-- Here are some test data
-- First insert new zone
INSERT INTO zones (zone_name) VALUES ('internal.');
INSERT INTO zones (zone_name) VALUES ('in-addr.arpa.');

-- Second insert records
INSERT INTO records (zone_id, hostname, type, data, ttl, online) VALUES 
    (1, '@', 'SOA', 'ns1.internal. root.internal. 1 3600 300 86400 300', 3600, 1),
    (1, '@', 'NS', 'ns1.internal.', 3600, 1),
    (1, 'ns1', 'A', '127.0.0.1', 3600, 1),
    (1, 'ns1', 'AAAA', '::1', 3600, 1),
    (1, 'www', 'A', '172.16.0.100', 120, 1),
    (1, 'web', 'CNAME', 'www.internal.', 60, 1),
    (2, '100.0.16.172', 'PTR', 'www.internal.', 120, 1);

~~~

test
~~~ bash
dig @127.0.0.1 internal SOA
dig @127.0.0.1 internal NS
dig @127.0.0.1 ns1.internal A
dig @127.0.0.1 ns1.internal AAAA
dig @127.0.0.1 www.internal A
dig @127.0.0.1 web.internal CNAME
# Support CNAME to A record query
dig @127.0.0.1 web.internal A
dig @127.0.0.1 -x 172.16.0.100

~~~

## Also See

See the [manual](https://coredns.io/manual).
