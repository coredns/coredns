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
	// FieldActivated holds the string denoting the activated field in the database.
	FieldActivated = "activated"
	// Table holds the table name of the dnsrr in the database.
	Table = "dns_rrs"
)

// Columns holds all SQL columns for dnsrr fields.
var Columns = []string{
	FieldID,
	FieldCreatedAt,
	FieldUpdatedAt,
	FieldName,
	FieldActivated,
}

// ValidColumn reports if the column name is valid (part of the table columns).
func ValidColumn(column string) bool {
	for i := range Columns {
		if column == Columns[i] {
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
	// DefaultActivated holds the default value on creation for the "activated" field.
	DefaultActivated bool
	// DefaultID holds the default value on creation for the "id" field.
	DefaultID func() xid.ID
)
