package mysql

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

func (m *Mysql) rePing() {
	for {
		if err := m.db.Ping(); err != nil {
			time.Sleep(m.failHeartbeatTime)
			m.db.Close()
			newDB, err := m.openDB()
			if err == nil {
				m.db = newDB
			}
			logger.Errorf("Failed to ping database: %s", err)
			dbPingCount.With(prometheus.Labels{"status": "fail"}).Inc()
			continue
		}
		time.Sleep(m.successHeartbeatTime)

		logger.Debug("Success to ping database")
		dbPingCount.With(prometheus.Labels{"status": "success"}).Inc()
	}
}

func (m *Mysql) reGetZone() {
	for {
		zoneMap := make(map[string]int, 0)
		rows, err := m.db.Query(m.queryZoneSQL)
		if err != nil {
			logger.Errorf("Failed to query zones: %s", err)
			dbGetZoneCount.With(prometheus.Labels{"status": "fail"}).Inc()

			time.Sleep(m.failHeartbeatTime)
			continue
		}

		for rows.Next() {
			var zoneRecord zoneRecord
			err := rows.Scan(&zoneRecord.id, &zoneRecord.name)
			if err != nil {
				logger.Error(err)
			}
			zoneMap[zoneRecord.name] = zoneRecord.id
		}
		m.zoneMap = zoneMap
		logger.Debugf("Success to query zones: %#v", zoneMap)
		dbGetZoneCount.With(prometheus.Labels{"status": "success"}).Inc()

		time.Sleep(m.successHeartbeatTime)
	}
}

func (m *Mysql) loadLocalData() {
	cache := make(map[record]dnsRecordInfo, zero)
	m.degradeCache = cache
	pureRecords := make([]pureRecord, zero)
	content, err := os.ReadFile(m.dumpFile)
	if err != nil {
		logger.Errorf("Failed to load data from file: %s", err)
		loadLocalData.With(prometheus.Labels{"status": "fail"}).Inc()

		return
	}
	err = json.Unmarshal(content, &pureRecords)
	if err != nil {
		logger.Errorf("Failed to load data from file: %s", err)
		loadLocalData.With(prometheus.Labels{"status": "fail"}).Inc()

		return
	}

	for _, rMap := range pureRecords {
		for queryKey, rrStrings := range rMap {
			var response []dns.RR
			queryKeySlice := strings.Split(queryKey, keySeparator)
			fqdn, qType := queryKeySlice[0], queryKeySlice[1]
			record := record{fqdn: fqdn, qType: qType}
			for _, rrString := range rrStrings {
				rr, err := dns.NewRR(rrString)
				if err != nil {
					continue
				}
				response = append(response, rr)
			}
			dnsRecordInfo := dnsRecordInfo{rrStrings: rrStrings, response: response}
			cache[record] = dnsRecordInfo
		}
	}
	// TODO add lock
	logger.Debugf("Load degrade data from local file %#v", cache)
	loadLocalData.With(prometheus.Labels{"status": "success"}).Inc()
	m.degradeCache = cache
}

func (m *Mysql) dump2LocalData() {
	pureRecord := make([]pureRecord, zero)
	for record, dnsRecordInfo := range m.degradeCache {
		logger.Debugf("Record %#v", record)
		pureRecord = append(pureRecord, map[string][]string{
			fmt.Sprintf("%s%s%s", record.fqdn, keySeparator, record.qType): dnsRecordInfo.rrStrings,
		})
	}

	content, err := json.Marshal(pureRecord)
	if err != nil {
		logger.Errorf("Failed to dump data to local: %s", err)
		dumpLocalData.With(prometheus.Labels{"status": "fail"}).Inc()
		return
	}
	if err := os.WriteFile(m.dumpFile, content, safeMode); err != nil {
		logger.Error(err)
		logger.Errorf("Failed to dump data to local: %s", err)
		dumpLocalData.With(prometheus.Labels{"status": "fail"}).Inc()
		return
	}
	logger.Debugf("Success to dump data to local: %#v", pureRecord)
	dumpLocalData.With(prometheus.Labels{"status": "success"}).Inc()
}

func (m *Mysql) openDB() (*sql.DB, error) {
	db, err := sql.Open("mysql", m.dsn)
	if err != nil {
		openMysqlCount.With(prometheus.Labels{"status": "fail"}).Inc()
		logger.Errorf("Failed to open database: %s", err)
	} else {
		// Config db connection pool
		db.SetConnMaxIdleTime(m.connMaxIdleTime)
		db.SetConnMaxLifetime(m.connMaxLifetime)
		db.SetMaxIdleConns(m.maxIdleConns)
		db.SetMaxOpenConns(m.maxOpenConns)
		openMysqlCount.With(prometheus.Labels{"status": "success"}).Inc()
		logger.Debug("Success to open database")
	}
	return db, err
}

func (m *Mysql) onStartup() error {
	logger.Debug("On start up")
	// Initialize database connection pool
	db, _ := m.openDB()

	m.db = db

	// Start rePing loop
	go m.rePing()
	// start reGetZone loop
	go m.reGetZone()
	// Load local file data
	m.loadLocalData()
	// Create tables
	m.createTables()
	return nil
}

func (m *Mysql) onShutdown() error {
	logger.Debug("on shutdown")
	if m.db != nil {
		m.db.Close()
	}
	// Dump memory data to local file
	m.dump2LocalData()
	return nil
}
