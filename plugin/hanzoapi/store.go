package hanzoapi

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RecordType enumerates supported DNS record types.
type RecordType string

const (
	TypeA     RecordType = "A"
	TypeAAAA  RecordType = "AAAA"
	TypeCNAME RecordType = "CNAME"
	TypeMX    RecordType = "MX"
	TypeTXT   RecordType = "TXT"
	TypeSRV   RecordType = "SRV"
	TypeNS    RecordType = "NS"
	TypeSOA   RecordType = "SOA"
	TypeCAA   RecordType = "CAA"
)

// ValidRecordTypes is the set of record types accepted by the API.
var ValidRecordTypes = map[RecordType]bool{
	TypeA: true, TypeAAAA: true, TypeCNAME: true, TypeMX: true,
	TypeTXT: true, TypeSRV: true, TypeNS: true, TypeSOA: true, TypeCAA: true,
}

// Record represents a single DNS record.
type Record struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Type      RecordType `json:"type"`
	TTL       uint32     `json:"ttl"`
	Content   string     `json:"content"`
	Priority  uint16     `json:"priority,omitempty"`
	Proxied   bool       `json:"proxied"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Zone represents a DNS zone and its records.
type Zone struct {
	ID            string    `json:"id"`
	Name          string    `json:"zone"`
	Status        string    `json:"status"`
	Nameservers   []string  `json:"nameservers"`
	RecordCount   int       `json:"record_count"`
	DNSSECEnabled bool      `json:"dnssec_enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Store provides thread-safe in-memory storage for DNS zones and records.
type Store struct {
	mu      sync.RWMutex
	zones   map[string]*zoneData // keyed by normalized zone name (e.g. "example.com.")
}

type zoneData struct {
	zone    Zone
	records map[string]*Record // keyed by record ID
}

// NewStore creates an empty Store.
func NewStore() *Store {
	return &Store{
		zones: make(map[string]*zoneData),
	}
}

// normZone ensures the zone name ends with a dot (FQDN).
func normZone(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if !strings.HasSuffix(name, ".") {
		name += "."
	}
	return name
}

// ListZones returns all zones.
func (s *Store) ListZones() []Zone {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Zone, 0, len(s.zones))
	for _, zd := range s.zones {
		z := zd.zone
		z.RecordCount = len(zd.records)
		out = append(out, z)
	}
	return out
}

// GetZone returns a zone by name or nil if not found.
func (s *Store) GetZone(name string) *Zone {
	s.mu.RLock()
	defer s.mu.RUnlock()

	zd, ok := s.zones[normZone(name)]
	if !ok {
		return nil
	}
	z := zd.zone
	z.RecordCount = len(zd.records)
	return &z
}

// CreateZone adds a new zone. Returns error if it already exists.
func (s *Store) CreateZone(name string) (*Zone, error) {
	key := normZone(name)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.zones[key]; exists {
		return nil, fmt.Errorf("zone %q already exists", key)
	}

	now := time.Now().UTC()
	z := Zone{
		ID:          uuid.New().String(),
		Name:        key,
		Status:      "active",
		Nameservers: []string{"ns1.hanzo.ai.", "ns2.hanzo.ai."},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.zones[key] = &zoneData{
		zone:    z,
		records: make(map[string]*Record),
	}
	return &z, nil
}

// DeleteZone removes a zone and all its records.
func (s *Store) DeleteZone(name string) error {
	key := normZone(name)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.zones[key]; !exists {
		return fmt.Errorf("zone %q not found", key)
	}
	delete(s.zones, key)
	return nil
}

// ListRecords returns all records for a zone.
func (s *Store) ListRecords(zone string) ([]Record, error) {
	key := normZone(zone)

	s.mu.RLock()
	defer s.mu.RUnlock()

	zd, ok := s.zones[key]
	if !ok {
		return nil, fmt.Errorf("zone %q not found", key)
	}

	out := make([]Record, 0, len(zd.records))
	for _, r := range zd.records {
		out = append(out, *r)
	}
	return out, nil
}

// GetRecord returns a single record by zone and ID.
func (s *Store) GetRecord(zone, id string) (*Record, error) {
	key := normZone(zone)

	s.mu.RLock()
	defer s.mu.RUnlock()

	zd, ok := s.zones[key]
	if !ok {
		return nil, fmt.Errorf("zone %q not found", key)
	}
	r, ok := zd.records[id]
	if !ok {
		return nil, fmt.Errorf("record %q not found", id)
	}
	cp := *r
	return &cp, nil
}

// CreateRecord adds a record to a zone. Returns the created record.
func (s *Store) CreateRecord(zone string, name string, rtype RecordType, ttl uint32, content string, priority uint16, proxied bool) (*Record, error) {
	key := normZone(zone)

	if !ValidRecordTypes[rtype] {
		return nil, fmt.Errorf("invalid record type %q", rtype)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	zd, ok := s.zones[key]
	if !ok {
		return nil, fmt.Errorf("zone %q not found", key)
	}

	now := time.Now().UTC()
	r := &Record{
		ID:        uuid.New().String(),
		Name:      name,
		Type:      rtype,
		TTL:       ttl,
		Content:   content,
		Priority:  priority,
		Proxied:   proxied,
		CreatedAt: now,
		UpdatedAt: now,
	}
	zd.records[r.ID] = r

	zd.zone.UpdatedAt = now

	cp := *r
	return &cp, nil
}

// UpdateRecord patches a record. Only non-zero fields are updated.
func (s *Store) UpdateRecord(zone, id string, patch RecordPatch) (*Record, error) {
	key := normZone(zone)

	s.mu.Lock()
	defer s.mu.Unlock()

	zd, ok := s.zones[key]
	if !ok {
		return nil, fmt.Errorf("zone %q not found", key)
	}
	r, ok := zd.records[id]
	if !ok {
		return nil, fmt.Errorf("record %q not found", id)
	}

	if patch.Name != nil {
		r.Name = *patch.Name
	}
	if patch.Type != nil {
		if !ValidRecordTypes[*patch.Type] {
			return nil, fmt.Errorf("invalid record type %q", *patch.Type)
		}
		r.Type = *patch.Type
	}
	if patch.TTL != nil {
		r.TTL = *patch.TTL
	}
	if patch.Content != nil {
		r.Content = *patch.Content
	}
	if patch.Priority != nil {
		r.Priority = *patch.Priority
	}
	if patch.Proxied != nil {
		r.Proxied = *patch.Proxied
	}
	r.UpdatedAt = time.Now().UTC()
	zd.zone.UpdatedAt = r.UpdatedAt

	cp := *r
	return &cp, nil
}

// DeleteRecord removes a record from a zone.
func (s *Store) DeleteRecord(zone, id string) error {
	key := normZone(zone)

	s.mu.Lock()
	defer s.mu.Unlock()

	zd, ok := s.zones[key]
	if !ok {
		return fmt.Errorf("zone %q not found", key)
	}
	if _, ok := zd.records[id]; !ok {
		return fmt.Errorf("record %q not found", id)
	}
	delete(zd.records, id)
	zd.zone.UpdatedAt = time.Now().UTC()
	return nil
}

// RecordPatch carries optional fields for a partial record update.
type RecordPatch struct {
	Name     *string     `json:"name,omitempty"`
	Type     *RecordType `json:"type,omitempty"`
	TTL      *uint32     `json:"ttl,omitempty"`
	Content  *string     `json:"content,omitempty"`
	Priority *uint16     `json:"priority,omitempty"`
	Proxied  *bool       `json:"proxied,omitempty"`
}

// Lookup returns all records matching the given FQDN and record type string.
// Used by the DNS handler to serve queries from the store.
func (s *Store) Lookup(qname string, qtype string) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()

	qname = strings.ToLower(qname)

	var matches []Record
	for zoneName, zd := range s.zones {
		if !strings.HasSuffix(qname, zoneName) {
			continue
		}
		for _, r := range zd.records {
			fqdn := s.fqdn(r.Name, zoneName)
			if strings.ToLower(fqdn) == qname && string(r.Type) == qtype {
				matches = append(matches, *r)
			}
		}
	}
	return matches
}

// ZoneNames returns all zone names the store knows about (FQDNs with trailing dot).
func (s *Store) ZoneNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.zones))
	for k := range s.zones {
		names = append(names, k)
	}
	return names
}

// fqdn converts a record name relative to a zone into an FQDN.
func (s *Store) fqdn(name, zone string) string {
	if name == "@" || name == "" {
		return zone
	}
	if strings.HasSuffix(name, ".") {
		return name
	}
	return name + "." + zone
}
