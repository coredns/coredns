package mysql

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"

	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	_ "github.com/go-sql-driver/mysql"
	"github.com/miekg/dns"
)

var logger = clog.NewWithPlugin(pluginName)

func (m *Mysql) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	var records []record
	state := request.Request{W: w, Req: r}
	answers := make([]dns.RR, 0)
	rrStrings := make([]string, 0)

	// Get domain name
	qName := state.Name()
	qType := state.Type()
	degradeRecord := record{fqdn: qName, qType: qType}

	logger.Debugf("New query: FQDN %s type %s", qName, qType)

	// Query zone cache
	zoneID, host, zone, err := m.getDomainInfo(qName)

	// Zone not exist, maybe db error cause no zone, goto degrade entrypoint
	if err != nil {
		goto DegradeEntrypoint
	}

	// Query DB, full match
	records, err = m.getRecords(zoneID, host, zone, qType)
	if err != nil {
		goto DegradeEntrypoint
	}

	// Try query CNAME type of record
	if len(records) == zero {
		cnameRecords, err := m.getRecords(zoneID, host, zone, cnameQtype)
		if err != nil {
			goto DegradeEntrypoint
		}
		for _, cnameRecord := range cnameRecords {
			cnameZoneID, cnameHost, cnameZone, err := m.getDomainInfo(cnameRecord.data)

			if err != nil {
				goto DegradeEntrypoint
			}

			rrString := fmt.Sprintf("%s %d IN %s %s", qName, cnameRecord.ttl, cnameRecord.qType, cnameRecord.data)
			rrStrings = append(rrStrings, rrString)
			rr, err := m.makeAnswer(rrString)
			if err != nil {
				continue
			}
			answers = append(answers, rr)

			cname2Records, err := m.getRecords(cnameZoneID, cnameHost, cnameZone, qType)

			if err != nil {
				goto DegradeEntrypoint
			}

			for _, cname2Record := range cname2Records {
				rrString := fmt.Sprintf("%s %d IN %s %s", cname2Record.fqdn, cname2Record.ttl, cname2Record.qType, cname2Record.data)
				rrStrings = append(rrStrings, rrString)
				rr, err := m.makeAnswer(rrString)
				if err != nil {
					continue
				}
				answers = append(answers, rr)
			}
		}
	}

	// Process records
	for _, record := range records {
		rrString := fmt.Sprintf("%s %d IN %s %s", record.fqdn, record.ttl, record.qType, record.data)
		rrStrings = append(rrStrings, rrString)
		rr, err := m.makeAnswer(rrString)
		if err != nil {
			continue
		}
		answers = append(answers, rr)
	}

	// Handle wildcard domains
	if len(answers) == zero && strings.Count(qName, zoneSeparator) > 1 {
		baseZone := m.getBaseZone(qName)
		zoneID, ok := m.getZoneID(baseZone)
		wildcardName := wildcard + zoneSeparator + baseZone
		if !ok {
			logger.Debugf("Failed to get zone %s from database: %s", qName, err)
			goto DegradeEntrypoint
		}
		records, err := m.getRecords(zoneID, wildcard, zone, qType)
		if err != nil {
			logger.Debugf("Failed to get records for domain %s from database: %s", wildcardName, err)
			goto DegradeEntrypoint
		}

		for _, record := range records {
			rrString := fmt.Sprintf("%s %d IN %s %s", qName, record.ttl, record.qType, record.data)
			rr, err := m.makeAnswer(rrString)
			rrStrings = append(rrStrings, rrString)
			if err != nil {
				continue
			}
			answers = append(answers, rr)
		}
	}

	// Common Entrypoint
	if len(answers) > zero {
		msg := MakeMessage(r, answers)
		w.WriteMsg(msg)
		dnsRecordInfo := dnsRecordInfo{rrStrings: rrStrings, response: answers}
		if cacheDnsRecordResponse, ok := m.degradeQuery(degradeRecord); !ok || !reflect.DeepEqual(cacheDnsRecordResponse, dnsRecordInfo.response) {
			m.degradeWrite(degradeRecord, dnsRecordInfo)
			logger.Debugf("CommonEntrypoint Add degrade record %#v, dnsRecordInfo %#v", degradeRecord, dnsRecordInfo)
			degradeCacheCount.With(prometheus.Labels{"status": "success", "option": "update", "fqdn": degradeRecord.fqdn, "qtype": degradeRecord.qType}).Inc()
			return dns.RcodeSuccess, nil
		}
		return dns.RcodeSuccess, nil
	}
	logger.Debug("Call next plugin")
	return plugin.NextOrFailure(m.Name(), m.Next, ctx, w, r)

	// Degrade Entrypoint
DegradeEntrypoint:
	if answers, ok := m.degradeQuery(degradeRecord); ok {
		msg := MakeMessage(r, answers)
		w.WriteMsg(msg)
		logger.Debugf("DegradeEntrypoint: Query degrade record %#v", degradeRecord)
		return dns.RcodeSuccess, nil
	}
	logger.Debug("Call next plugin")
	callNextPluginCount.With(prometheus.Labels{"fqdn": qName, "qtype": qType}).Inc()
	return plugin.NextOrFailure(m.Name(), m.Next, ctx, w, r)
}
