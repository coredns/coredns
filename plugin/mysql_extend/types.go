package coredns_mysql_extend

import (
	"database/sql"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
)

type Mysql struct {
	*mysqlConfig

	degradeCache map[record]dnsRecordInfo
	zoneMap      map[string]int

	Next plugin.Handler
	db   *sql.DB
}

type pureRecord map[string][]string

type mysqlConfig struct {
	dsn          string
	dumpFile     string
	ttl          uint32
	zonesTable   string
	recordsTable string

	maxIdleConns    int
	maxOpenConns    int
	connMaxIdleTime time.Duration
	connMaxLifetime time.Duration

	failHeartbeatTime    time.Duration
	successHeartbeatTime time.Duration

	queryZoneSQL   string
	queryRecordSQL string
}

type dnsRecordInfo struct {
	response  []dns.RR
	rrStrings []string
}

type zoneRecord struct {
	id   int
	name string
}

type record struct {
	id       int
	zoneID   int
	zoneName string
	name     string
	qType    string
	data     string
	fqdn     string
	ttl      uint32
}
