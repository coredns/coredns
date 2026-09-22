package dynupdate

import (
	"fmt"

	"github.com/miekg/dns"
)

const (
	defaultMaxRecords       = 10000
	defaultMaxBytes         = 8 << 20
	defaultMaxUpdateRecords = 1024
)

type limits struct {
	records, bytes, updateRecords int
}

func (l limits) defaults() limits {
	if l.records == 0 {
		l.records = defaultMaxRecords
	}
	if l.bytes == 0 {
		l.bytes = defaultMaxBytes
	}
	if l.updateRecords == 0 {
		l.updateRecords = defaultMaxUpdateRecords
	}
	return l
}

func (l limits) check(records []dns.RR) error {
	l = l.defaults()
	if len(records) > l.records {
		return fmt.Errorf("zone exceeds max_records (%d)", l.records)
	}
	remaining := l.bytes
	for _, rr := range records {
		if rr == nil {
			return fmt.Errorf("zone contains a nil record")
		}
		size := dns.Len(rr)
		if size > remaining {
			return fmt.Errorf("zone exceeds max_bytes (%d)", l.bytes)
		}
		remaining -= size
	}
	return nil
}
