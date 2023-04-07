package coredns_mysql_extend

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

func MakeMysqlPlugin() *Mysql {
	return &Mysql{}
}

func MakeMessage(r *dns.Msg, answers []dns.RR) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Answer = answers
	return msg
}

func (m *Mysql) getDomainInfo(fqdn string) (int, string, string, error) {
	var (
		id    int
		host  string
		ok    bool
		zone  = fqdn
		items = strings.Split(zone, zoneSeparator)
	)

	// Only should case once but more. TODO
	for i := range items {
		zone = strings.Join(items[i:], zoneSeparator)
		id, ok = m.getZoneID(zone)
		host = strings.Join(items[:i], zoneSeparator)
		if host == "" {
			host = zoneSelf
		}
		if ok {
			logger.Debugf("Query zone %s in zone cache", zone)
			zoneFindCount.With(prometheus.Labels{"status": "success"}).Inc()
			return id, host, zone, nil
		}
	}
	logger.Warningf("Query zone %s not in zone cache, fqdn: %s", zone, fqdn)
	zoneFindCount.With(prometheus.Labels{"status": "fail"}).Inc()
	return id, host, zone, fmt.Errorf("zone %s not exist", fqdn)
}

func (m *Mysql) getZoneID(zone string) (int, bool) {
	id, ok := m.zoneMap[zone]
	return id, ok
}

func (m *Mysql) getBaseZone(fqdn string) string {
	if strings.Count(fqdn, zoneSeparator) > 1 {
		return strings.Join(strings.Split(fqdn, zoneSeparator)[1:], zoneSeparator)
	}
	return rootZone
}

func (m *Mysql) degradeQuery(record record) ([]dns.RR, bool) {
	dnsRecordInfo, ok := m.degradeCache[record]
	if !ok {
		degradeCacheCount.With(prometheus.Labels{"option": "query", "status": "fail", "fqdn": record.fqdn, "qtype": record.qType}).Inc()
	} else {
		degradeCacheCount.With(prometheus.Labels{"option": "query", "status": "success", "fqdn": record.fqdn, "qtype": record.qType}).Inc()
	}
	return dnsRecordInfo.response, ok
}

func (m *Mysql) degradeWrite(record record, dnsRecordInfo dnsRecordInfo) {
	m.degradeCache[record] = dnsRecordInfo
}

func (m *Mysql) getRecords(zoneID int, host, zone, qType string) ([]record, error) {
	var records []record

	rows := m.db.QueryRow(m.queryRecordSQL, zoneID, host, qType)

	for {
		var record record
		err := rows.Scan(&record.id, &record.zoneID, &record.name, &record.qType, &record.data, &record.ttl)
		if err == sql.ErrNoRows {
			queryDBCount.With(prometheus.Labels{"status": "success"}).Inc()
			logger.Debugf("Query records in db used zone id %d, host %s, zone %s, type %s, records %#v", zoneID, host, zone, qType, records)

			return records, nil
		}
		if err != nil {
			queryDBCount.With(prometheus.Labels{"status": "fail"}).Inc()
			logger.Debugf("Failed to get records for domain %s from database: %s", record.fqdn, err)
			return nil, err
		}
		record.zoneName = zone
		if host == zoneSelf {
			record.fqdn = record.zoneName
		} else {
			record.fqdn = record.name + zoneSeparator + record.zoneName
		}
		records = append(records, record)
	}
}

func (m *Mysql) makeAnswer(rrString string) (dns.RR, error) {
	rr, err := dns.NewRR(rrString)
	if err != nil {
		makeAnswerCount.With(prometheus.Labels{"status": "fail"}).Inc()
		logger.Errorf("Failed to create DNS record: %s", err)
	} else {
		makeAnswerCount.With(prometheus.Labels{"status": "success"}).Inc()
	}
	return rr, nil
}
