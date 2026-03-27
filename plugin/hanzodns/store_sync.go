package hanzodns

import (
	"time"

	"github.com/google/uuid"
)

// SyncZoneRequest represents a bulk zone sync payload.
type SyncZoneRequest struct {
	Zone    string       `json:"zone"`
	OrgID   string       `json:"org_id,omitempty"`
	Records []SyncRecord `json:"records"`
}

// SyncRecord is a record in a bulk sync request.
type SyncRecord struct {
	ID       string     `json:"id,omitempty"`
	Name     string     `json:"name"`
	Type     RecordType `json:"type"`
	TTL      uint32     `json:"ttl"`
	Content  string     `json:"content"`
	Priority uint16     `json:"priority,omitempty"`
	Proxied  bool       `json:"proxied"`
}

// SyncResponse is returned from the bulk sync endpoint.
type SyncResponse struct {
	Zone        string `json:"zone"`
	RecordCount int    `json:"record_count"`
	Created     int    `json:"created"`
	Deleted     int    `json:"deleted"`
}

// BulkSync atomically replaces all records for a zone. If the zone does not
// exist it is created first. Returns counts of records created and deleted.
func (s *Store) BulkSync(req SyncZoneRequest) (*SyncResponse, error) {
	key := normZone(req.Zone)

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()

	zd, exists := s.zones[key]
	if !exists {
		zd = &zoneData{
			zone: Zone{
				ID:          uuid.New().String(),
				Name:        key,
				Status:      "active",
				Nameservers: []string{"ns1.hanzo.ai.", "ns2.hanzo.ai."},
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			records: make(map[string]*Record),
		}
		s.zones[key] = zd
	}

	deleted := len(zd.records)

	// Replace all records atomically.
	newRecords := make(map[string]*Record, len(req.Records))
	for _, sr := range req.Records {
		if !ValidRecordTypes[sr.Type] {
			continue
		}
		id := sr.ID
		if id == "" {
			id = uuid.New().String()
		}
		ttl := sr.TTL
		if ttl == 0 {
			ttl = 300
		}
		newRecords[id] = &Record{
			ID:        id,
			Name:      sr.Name,
			Type:      sr.Type,
			TTL:       ttl,
			Content:   sr.Content,
			Priority:  sr.Priority,
			Proxied:   sr.Proxied,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	zd.records = newRecords
	zd.zone.UpdatedAt = now

	return &SyncResponse{
		Zone:        key,
		RecordCount: len(newRecords),
		Created:     len(newRecords),
		Deleted:     deleted,
	}, nil
}
