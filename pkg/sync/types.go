package sync

import (
	"database/sql"
	"time"
)

// DBZone represents a row from the dns_zones table.
type DBZone struct {
	ID               string
	OrgID            string
	Name             string
	Status           string
	CloudflareZoneID sql.NullString
	UpdatedAt        time.Time
}

// DBRecord represents a row from the dns_records table.
type DBRecord struct {
	ID                 string
	ZoneID             string
	Name               string
	Type               string
	Content            string
	TTL                int
	Priority           sql.NullInt32
	Proxied            bool
	SyncToCloudflare   bool
	CloudflareRecordID sql.NullString
	UpdatedAt          time.Time
}

// zoneWithRecords pairs a zone with its records for sync.
type zoneWithRecords struct {
	Zone    DBZone
	Records []DBRecord
}
