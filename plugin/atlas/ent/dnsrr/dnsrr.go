// Code generated by ent, DO NOT EDIT.

package dnsrr

import (
	"time"

	"github.com/rs/xid"
)

const (
	// Label holds the string label denoting the dnsrr type in the database.
	Label = "dns_rr"
	// FieldID holds the string denoting the id field in the database.
	FieldID = "id"
	// FieldCreatedAt holds the string denoting the created_at field in the database.
	FieldCreatedAt = "created_at"
	// FieldUpdatedAt holds the string denoting the updated_at field in the database.
	FieldUpdatedAt = "updated_at"
	// FieldName holds the string denoting the name field in the database.
	FieldName = "name"
	// FieldRrtype holds the string denoting the rrtype field in the database.
	FieldRrtype = "rrtype"
	// FieldRrcontent holds the string denoting the rrcontent field in the database.
	FieldRrcontent = "rrcontent"
	// FieldClass holds the string denoting the class field in the database.
	FieldClass = "class"
	// FieldTTL holds the string denoting the ttl field in the database.
	FieldTTL = "ttl"
	// FieldRdlength holds the string denoting the rdlength field in the database.
	FieldRdlength = "rdlength"
	// FieldActivated holds the string denoting the activated field in the database.
	FieldActivated = "activated"
	// EdgeZone holds the string denoting the zone edge name in mutations.
	EdgeZone = "zone"
	// Table holds the table name of the dnsrr in the database.
	Table = "dns_rrs"
	// ZoneTable is the table that holds the zone relation/edge.
	ZoneTable = "dns_rrs"
	// ZoneInverseTable is the table name for the DnsZone entity.
	// It exists in this package in order to avoid circular dependency with the "dnszone" package.
	ZoneInverseTable = "dns_zones"
	// ZoneColumn is the table column denoting the zone relation/edge.
	ZoneColumn = "dns_zone_records"
)

// Columns holds all SQL columns for dnsrr fields.
var Columns = []string{
	FieldID,
	FieldCreatedAt,
	FieldUpdatedAt,
	FieldName,
	FieldRrtype,
	FieldRrcontent,
	FieldClass,
	FieldTTL,
	FieldRdlength,
	FieldActivated,
}

// ForeignKeys holds the SQL foreign-keys that are owned by the "dns_rrs"
// table and are not defined as standalone fields in the schema.
var ForeignKeys = []string{
	"dns_zone_records",
}

// ValidColumn reports if the column name is valid (part of the table columns).
func ValidColumn(column string) bool {
	for i := range Columns {
		if column == Columns[i] {
			return true
		}
	}
	for i := range ForeignKeys {
		if column == ForeignKeys[i] {
			return true
		}
	}
	return false
}

var (
	// DefaultCreatedAt holds the default value on creation for the "created_at" field.
	DefaultCreatedAt func() time.Time
	// DefaultUpdatedAt holds the default value on creation for the "updated_at" field.
	DefaultUpdatedAt func() time.Time
	// UpdateDefaultUpdatedAt holds the default value on update for the "updated_at" field.
	UpdateDefaultUpdatedAt func() time.Time
	// NameValidator is a validator for the "name" field. It is called by the builders before save.
	NameValidator func(string) error
	// DefaultClass holds the default value on creation for the "class" field.
	DefaultClass uint16
	// DefaultTTL holds the default value on creation for the "ttl" field.
	DefaultTTL uint32
	// TTLValidator is a validator for the "ttl" field. It is called by the builders before save.
	TTLValidator func(uint32) error
	// DefaultActivated holds the default value on creation for the "activated" field.
	DefaultActivated bool
	// DefaultID holds the default value on creation for the "id" field.
	DefaultID func() xid.ID
)
