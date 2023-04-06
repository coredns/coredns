// Code generated by ent, DO NOT EDIT.

package ent

import (
	"fmt"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/coredns/coredns/plugin/atlas/ent/dnsrr"
)

// DnsRR is the model entity for the DnsRR schema.
type DnsRR struct {
	config
	// ID of the ent.
	ID int `json:"id,omitempty"`
}

// scanValues returns the types for scanning values from sql.Rows.
func (*DnsRR) scanValues(columns []string) ([]any, error) {
	values := make([]any, len(columns))
	for i := range columns {
		switch columns[i] {
		case dnsrr.FieldID:
			values[i] = new(sql.NullInt64)
		default:
			return nil, fmt.Errorf("unexpected column %q for type DnsRR", columns[i])
		}
	}
	return values, nil
}

// assignValues assigns the values that were returned from sql.Rows (after scanning)
// to the DnsRR fields.
func (dr *DnsRR) assignValues(columns []string, values []any) error {
	if m, n := len(values), len(columns); m < n {
		return fmt.Errorf("mismatch number of scan values: %d != %d", m, n)
	}
	for i := range columns {
		switch columns[i] {
		case dnsrr.FieldID:
			value, ok := values[i].(*sql.NullInt64)
			if !ok {
				return fmt.Errorf("unexpected type %T for field id", value)
			}
			dr.ID = int(value.Int64)
		}
	}
	return nil
}

// Update returns a builder for updating this DnsRR.
// Note that you need to call DnsRR.Unwrap() before calling this method if this DnsRR
// was returned from a transaction, and the transaction was committed or rolled back.
func (dr *DnsRR) Update() *DnsRRUpdateOne {
	return NewDnsRRClient(dr.config).UpdateOne(dr)
}

// Unwrap unwraps the DnsRR entity that was returned from a transaction after it was closed,
// so that all future queries will be executed through the driver which created the transaction.
func (dr *DnsRR) Unwrap() *DnsRR {
	_tx, ok := dr.config.driver.(*txDriver)
	if !ok {
		panic("ent: DnsRR is not a transactional entity")
	}
	dr.config.driver = _tx.drv
	return dr
}

// String implements the fmt.Stringer.
func (dr *DnsRR) String() string {
	var builder strings.Builder
	builder.WriteString("DnsRR(")
	builder.WriteString(fmt.Sprintf("id=%v", dr.ID))
	builder.WriteByte(')')
	return builder.String()
}

// DnsRRs is a parsable slice of DnsRR.
type DnsRRs []*DnsRR
