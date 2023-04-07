package mysql

import "time"

const (
	defaultDSN                  = "username:password@tcp(127.0.0.1:3306)/dns"
	defaultDumpFile             = "dump_dns.json"
	defaultTTL                  = 360
	defaultZonesTable           = "zones"
	defaultRecordsTable         = "records"
	defaultMaxIdleConns         = 4
	defaultMaxOpenConns         = 8
	defaultConnMaxIdleTime      = time.Hour * 1
	defaultConnMaxLifeTime      = time.Hour * 24
	defaultFailHeartBeatTime    = time.Second * 10
	defaultSuccessHeartBeatTime = time.Second * 60

	defaultQueryZoneSQL   = "SELECT id, zone_name FROM %s"
	defaultQueryRecordSQL = "SELECT id, zone_id, hostname, type, data, ttl FROM  %s WHERE online!=0 and zone_id=? and hostname=? and type=?"

	zero          = 0
	zeroTime      = zero
	safeMode      = 0640
	rootZone      = "."
	keySeparator  = ":"
	zoneSeparator = "."
	wildcard      = "*"
	zoneSelf      = "@"
	soaQtype      = "SOA"
	nsQtype       = "NS"
	cnameQtype    = "CNAME"
	pluginName    = "mysql_extend"
)
