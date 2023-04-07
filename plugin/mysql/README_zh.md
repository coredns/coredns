# mysql_extend

## Name

*mysql_extend* - 使用mysql作为DNS记录的后端存储. [English](./README.md) - 中文

## Description

mysql_extend 插件使用mysql作为DNS记录的后端存储. 此插件不依赖mysql的绝对高可用.

插件的其他特性: 
1. 插件使用连接池连接 DB, 可以复用底层的TCP连接, 极大的提高效率
2. 支持泛域名的查询
3. 支持递归查询
4. 支持上线功能, 只有在 db records 表中 online 字段非0时, 才能被响应. 可以配置 query_record_sql 取消这个特性
5. 支持 CNAME, A, AAAA, SOA, NS 等常见的 DNS 查询类型, 如果有问题欢迎向我提 issue
6. 不严重的依赖 mysql 的绝对高可用, 这在生产环境中是极其重要的. 此插件被设计可以从本地文件中加载数据(目前仅支持A和CNAME类型)
7. 丰富的指标信息, 可以让我们监控此插件的运行情况
8. 丰富的debug日志, 当出现任何问题是可以方便的排错. 同事也方便大家快捷的进行二次开发此插件
9. 如果连接的 mysql 上没有zone或record表, 那么会使用 `zone_tables` 和 `record_tables` 配置进行自动创建表


## Compilation

此包将始终作为 CoreDNS 的一部分进行编译，而不是以独立的方式进行编译. 需要你使用 `go get` 下载 plugin.cfg 中依赖的此插件包,  [plugin.cfg](https://github.com/coredns/coredns/blob/master/plugin.cfg).

这里 [manual](https://coredns.io/manual/toc/#what-is-coredns) 有更多关于如何使用外部插件配置和扩展服务器的信息

使用此插件的简单方法是通过向 plugin.cfg 中添加以下内容 [plugin.cfg](https://github.com/coredns/coredns/blob/master/plugin.cfg), 然后重新编译 [detailed on coredns.io](https://coredns.io/2017/07/25/compile-time-enabling-or-disabling-plugins/#build-with-compile-time-configuration-file).

~~~
mysql:github.com/snail2sky/coredns_mysql_extend
~~~

将其放在插件列表的前面，以便 *mysql_extend* 在任何其他插件之前执行

在此之后，您可以通过以下方式编译 coredns：

``` sh
go generate
go get
go build
```

## Syntax

~~~ txt
mysql {
    dsn username:password@tcp(127.0.0.1:3306)/dns
    # 以下为默认值, 如无自定义要求, 可以留空
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

- `dsn` <DSN>: 连接mysql的url, 符合dsn格式 详细细节可以查看 https://github.com/go-sql-driver/mysql#dsn-data-source-name. 默认值为 `username:password@tcp(127.0.0.1:3306)/dns`
- `dump_file` <FILE_PATH_STRING>: 使用此文件导入或导出数据, 如果DB出了问题, 那么这个特性就会非常有用. 默认值为 `dump_dns.json`
- `ttl` <TTL_INT>: 如果从DB中查询的ttl小于等于0, 那么就会使用此值. 默认值为 `360`
- `zones_table` <TABLE_NAME_STRING>: 存放zone信息的表名, 查询数据库病获取所有的 区域 zone, 然后这些 zone 会被缓存下来以提高效率. 默认值为 `zones`
- `records_table` <TABLE_NAME_STRING>: 存放所有记录的表明, 查询所有的记录.默认值为 `records`
- `db_max_idle_conns` <INT>: 设置db连接池的参数. 默认值为 `4`
- `db_max_open_conns` <INT>: 设置db连接池的参数. 默认值为 `8`
- `db_conn_max_idle_time` <TIME_DURATION>: 设置db连接池的参数. 默认值为 `1h`
- `db_conn_max_life_time` <TIME_DURATION>: 设置db连接池的参数. 默认值为 `24h`
- `fail_heartbeat_time` <TIME_DURATION>: 获取 zone 和 ping db 失败后 重做的时间间隔. 默认值为 `10s`
- `success_heartbeat_time` <TIME_DURATION>: 获取 zone 和 ping db 成功后 重做的时间间隔. 默认值为  `60s`
- `query_zone_sql` <SQL_FORMAT>: 设置查询DB的SQL, 如果你想优化sql可以修改此值. 默认值为 `"SELECT id, zone_name FROM %s"`
- `query_record_sql` <SQL_FORMAT>: 设置查询DB的SQL, 如果你想优化sql可以修改此值. 默认值为 `"SELECT id, zone_id, hostname, type, data, ttl FROM  %s WHERE online!=0 and zone_id=? and hostname=? and type=?"`

## Metrics

如果启用监控（通过 *prometheus* 指令），将导出以下指标：

* `open_mysql_total{status}` - 打开mysql实例的总数
* `create_table_total{status, table_name}` - 创建表的总数
* `degrade_cache_total{option, status, fqdn, qtype}` - 使用降级策略的次数, 一般DB出问题或查询过快会导致此指标飙升
* `zone_find_total{status}` - 从内存中获取zone的次数
* `call_next_plugin_total{fqdn, qtype}` - 调用下一个插件的总数, 一般此插件无法处理时会导致此指标飙升
* `query_db_total{status}` - 查询DB的总次数
* `make_answer_total{status}` - 创建一条记录的总次数
* `db_ping_total{status}` - ping DB的总次数
* `db_get_zone_total{status}` - 从DB中查询zone的总次数

`status` 标签将记录该指标对应的操作的状态
`table_name` 标签表明该指标对应的表名
`option` 标签表名该指标对应的操作
`fqdn` 标签表名该指标对应的 fqdn
`qtype` 标签表名该指标对应的 查询类型


## Examples

- 在此配置中, 我们将所有查询 以 internal 结尾的域名查询使用 本插件处理, 并用cache 插件以提高效率
- 建议: 将需要被查询的区域放到同一个 mysql 插件中, 否则需要更改dump_file指定的值, 防止对一个文件重复写导致数据不一致
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
-- 默认创建表的SQL
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

-- 插入一些测试数据
-- 插入测试zone数据
INSERT INTO zones (zone_name) VALUES ('internal.');
INSERT INTO zones (zone_name) VALUES ('in-addr.arpa.');

-- 插入测试records数据
INSERT INTO records (zone_id, hostname, type, data, ttl, online) VALUES 
    (1, '@', 'SOA', 'ns1.internal. root.internal. 1 3600 300 86400 300', 3600, 1),
    (1, '@', 'NS', 'ns1.internal.', 3600, 1),
    (1, 'ns1', 'A', '127.0.0.1', 3600, 1),
    (1, 'ns1', 'AAAA', '::1', 3600, 1),
    (1, 'www', 'A', '172.16.0.100', 120, 1),
    (1, 'web', 'CNAME', 'www.internal.', 60, 1),
    (2, '100.0.16.172', 'PTR', 'www.internal.', 120, 1);

~~~

测试
~~~ bash
dig @127.0.0.1 internal SOA
dig @127.0.0.1 internal NS
dig @127.0.0.1 ns1.internal A
dig @127.0.0.1 ns1.internal AAAA
dig @127.0.0.1 www.internal A
dig @127.0.0.1 web.internal CNAME
# 支持不存在A记录但存在CNAME的记录查询, 如 www.baidu.com -> www.a.shifen.com
dig @127.0.0.1 web.internal A
dig @127.0.0.1 -x 172.16.0.100

~~~

## Also See

详情查看 [manual](https://coredns.io/manual).
