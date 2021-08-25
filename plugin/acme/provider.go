package acme

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/libdns/libdns"
)

type Provider struct {
	recordsForZone map[string][]libdns.Record
	m              sync.Mutex
}

func (p *Provider) listAllRecords(ctx context.Context, zone string) []libdns.Record {
	records, found := p.recordsForZone[zone]
	if !found {
		return []libdns.Record{}
	}
	return records
}

func (p *Provider) AppendRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	p.m.Lock()
	defer p.m.Unlock()
	records := p.listAllRecords(ctx, zone)
	records = append(records, recs...)
	p.recordsForZone[zone] = records
	return recs, nil
}

func (p *Provider) DeleteRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	p.m.Lock()
	defer p.m.Unlock()
	deletedRecords := []libdns.Record{}
	remainingRecords := []libdns.Record{}
	records := p.listAllRecords(ctx, zone)
	shouldDelete := make([]bool, len(records))
	for _, deleteRecord := range recs {
		for i, record := range records {
			if reflect.DeepEqual(deleteRecord, record) {
				shouldDelete[i] = true
			}
		}
	}
	for i, record := range records {
		if !shouldDelete[i] {
			remainingRecords = append(remainingRecords, record)
		} else {
			deletedRecords = append(deletedRecords, record)
		}
	}
	p.recordsForZone[zone] = remainingRecords
	return deletedRecords, nil
}

func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	p.m.Lock()
	defer p.m.Unlock()
	records := p.listAllRecords(ctx, zone)
	if len(records) == 0 {
		return records, fmt.Errorf("no records were found for %v", zone)
	}
	return records, nil
}

var (
	_ libdns.RecordGetter   = (*Provider)(nil)
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
)
