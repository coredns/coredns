// Code generated by ent, DO NOT EDIT.

package ent

import (
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/coredns/coredns/plugin/atlas/ent/dnsrr"
	"github.com/rs/xid"
)

// DnsRR is the model entity for the DnsRR schema.
type DnsRR struct {
	config `json:"-"`
	// ID of the ent.
	// record identifier
	ID xid.ID `json:"id,omitempty"`
	// record creation date
	CreatedAt time.Time `json:"created_at,omitempty"`
	// record update date
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	// Name holds the value of the "name" field.
	Name string `json:"name,omitempty"`
	// only activated resource records will be served
	Activated bool `json:"activated,omitempty"`
}

// scanValues returns the types for scanning values from sql.Rows.
func (*DnsRR) scanValues(columns []string) ([]any, error) {
	values := make([]any, len(columns))
	for i := range columns {
		switch columns[i] {
		case dnsrr.FieldActivated:
			values[i] = new(sql.NullBool)
		case dnsrr.FieldName:
			values[i] = new(sql.NullString)
		case dnsrr.FieldCreatedAt, dnsrr.FieldUpdatedAt:
			values[i] = new(sql.NullTime)
		case dnsrr.FieldID:
			values[i] = new(xid.ID)
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
			if value, ok := values[i].(*xid.ID); !ok {
				return fmt.Errorf("unexpected type %T for field id", values[i])
			} else if value != nil {
				dr.ID = *value
			}
		case dnsrr.FieldCreatedAt:
			if value, ok := values[i].(*sql.NullTime); !ok {
				return fmt.Errorf("unexpected type %T for field created_at", values[i])
			} else if value.Valid {
				dr.CreatedAt = value.Time
			}
		case dnsrr.FieldUpdatedAt:
			if value, ok := values[i].(*sql.NullTime); !ok {
				return fmt.Errorf("unexpected type %T for field updated_at", values[i])
			} else if value.Valid {
				dr.UpdatedAt = value.Time
			}
		case dnsrr.FieldName:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field name", values[i])
			} else if value.Valid {
				dr.Name = value.String
			}
		case dnsrr.FieldActivated:
			if value, ok := values[i].(*sql.NullBool); !ok {
				return fmt.Errorf("unexpected type %T for field activated", values[i])
			} else if value.Valid {
				dr.Activated = value.Bool
			}
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
	builder.WriteString(fmt.Sprintf("id=%v, ", dr.ID))
	builder.WriteString("created_at=")
	builder.WriteString(dr.CreatedAt.Format(time.ANSIC))
	builder.WriteString(", ")
	builder.WriteString("updated_at=")
	builder.WriteString(dr.UpdatedAt.Format(time.ANSIC))
	builder.WriteString(", ")
	builder.WriteString("name=")
	builder.WriteString(dr.Name)
	builder.WriteString(", ")
	builder.WriteString("activated=")
	builder.WriteString(fmt.Sprintf("%v", dr.Activated))
	builder.WriteByte(')')
	return builder.String()
}

// DnsRRs is a parsable slice of DnsRR.
type DnsRRs []*DnsRR
