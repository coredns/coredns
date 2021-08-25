package acme

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/libdns/libdns"
)

func generateRandomRecords(n int) []libdns.Record {
	records := []libdns.Record{}
	for i := 0; i < n; i += 1 {
		records = append(records, libdns.Record{
			ID:    uuid.New().String(),
			Type:  uuid.New().String(),
			Name:  uuid.New().String(),
			Value: uuid.New().String(),
			TTL:   time.Minute,
		})
	}
	return records
}
func TestProviderAppendRecords(t *testing.T) {
	provider := Provider{
		recordsForZone: make(map[string][]libdns.Record),
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	type RecordsByZone struct {
		zone    string
		records []libdns.Record
	}
	recordsByZones := []RecordsByZone{}
	for i := 0; i < 3; i++ {
		recordsByZones = append(recordsByZones, RecordsByZone{
			zone:    uuid.New().String(),
			records: generateRandomRecords(5),
		})
	}
	wg.Add(len(recordsByZones))
	for _, recordsByZone := range recordsByZones {
		go func(zoneRecords RecordsByZone) {
			defer wg.Done()
			records, _ := provider.AppendRecords(ctx, zoneRecords.zone, zoneRecords.records)
			if !reflect.DeepEqual(zoneRecords.records, records) {
				t.Errorf("provider.AppendRecords: expected %+v but got %+v", zoneRecords.records, records)
			}
		}(recordsByZone)
	}
	wg.Wait()
	for _, recordsByZone := range recordsByZones {
		records, err := provider.GetRecords(ctx, recordsByZone.zone)
		if err != nil {
			t.Errorf("provider.GetRecords: %s", err.Error())
		}
		if !reflect.DeepEqual(recordsByZone.records, records) {
			t.Errorf("Expected %+v but got %+v", records, recordsByZone.records)
		}
	}
}
