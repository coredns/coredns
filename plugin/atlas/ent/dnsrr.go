// Code generated by ent, DO NOT EDIT.

package ent

import (
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/coredns/coredns/plugin/atlas/ent/dnsrr"
	"github.com/coredns/coredns/plugin/atlas/ent/dnszone"
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
	// resource record type
	Rrtype uint16 `json:"rrtype,omitempty"`
	// resource record content
	Rrcontent string `json:"rrcontent,omitempty"`
	// class
	Class uint16 `json:"class,omitempty"`
	// Time-to-live
	TTL uint32 `json:"ttl,omitempty"`
	// length of data after header
	Rdlength uint16 `json:"rdlength,omitempty"`
	// only activated resource records will be served
	Activated bool `json:"activated,omitempty"`
	// Edges holds the relations/edges for other nodes in the graph.
	// The values are being populated by the DnsRRQuery when eager-loading is set.
	Edges            DnsRREdges `json:"edges"`
	dns_zone_records *xid.ID
}

// DnsRREdges holds the relations/edges for other nodes in the graph.
type DnsRREdges struct {
	// Zone holds the value of the zone edge.
	Zone *DNSZone `json:"zone,omitempty"`
	// loadedTypes holds the information for reporting if a
	// type was loaded (or requested) in eager-loading or not.
	loadedTypes [1]bool
}

// ZoneOrErr returns the Zone value or an error if the edge
// was not loaded in eager-loading, or loaded but was not found.
func (e DnsRREdges) ZoneOrErr() (*DNSZone, error) {
	if e.loadedTypes[0] {
		if e.Zone == nil {
			// Edge was loaded but was not found.
			return nil, &NotFoundError{label: dnszone.Label}
		}
		return e.Zone, nil
	}
	return nil, &NotLoadedError{edge: "zone"}
}

// scanValues returns the types for scanning values from sql.Rows.
func (*DnsRR) scanValues(columns []string) ([]any, error) {
	values := make([]any, len(columns))
	for i := range columns {
		switch columns[i] {
		case dnsrr.FieldActivated:
			values[i] = new(sql.NullBool)
		case dnsrr.FieldRrtype, dnsrr.FieldClass, dnsrr.FieldTTL, dnsrr.FieldRdlength:
			values[i] = new(sql.NullInt64)
		case dnsrr.FieldName, dnsrr.FieldRrcontent:
			values[i] = new(sql.NullString)
		case dnsrr.FieldCreatedAt, dnsrr.FieldUpdatedAt:
			values[i] = new(sql.NullTime)
		case dnsrr.FieldID:
			values[i] = new(xid.ID)
		case dnsrr.ForeignKeys[0]: // dns_zone_records
			values[i] = &sql.NullScanner{S: new(xid.ID)}
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
		case dnsrr.FieldRrtype:
			if value, ok := values[i].(*sql.NullInt64); !ok {
				return fmt.Errorf("unexpected type %T for field rrtype", values[i])
			} else if value.Valid {
				dr.Rrtype = uint16(value.Int64)
			}
		case dnsrr.FieldRrcontent:
			if value, ok := values[i].(*sql.NullString); !ok {
				return fmt.Errorf("unexpected type %T for field rrcontent", values[i])
			} else if value.Valid {
				dr.Rrcontent = value.String
			}
		case dnsrr.FieldClass:
			if value, ok := values[i].(*sql.NullInt64); !ok {
				return fmt.Errorf("unexpected type %T for field class", values[i])
			} else if value.Valid {
				dr.Class = uint16(value.Int64)
			}
		case dnsrr.FieldTTL:
			if value, ok := values[i].(*sql.NullInt64); !ok {
				return fmt.Errorf("unexpected type %T for field ttl", values[i])
			} else if value.Valid {
				dr.TTL = uint32(value.Int64)
			}
		case dnsrr.FieldRdlength:
			if value, ok := values[i].(*sql.NullInt64); !ok {
				return fmt.Errorf("unexpected type %T for field rdlength", values[i])
			} else if value.Valid {
				dr.Rdlength = uint16(value.Int64)
			}
		case dnsrr.FieldActivated:
			if value, ok := values[i].(*sql.NullBool); !ok {
				return fmt.Errorf("unexpected type %T for field activated", values[i])
			} else if value.Valid {
				dr.Activated = value.Bool
			}
		case dnsrr.ForeignKeys[0]:
			if value, ok := values[i].(*sql.NullScanner); !ok {
				return fmt.Errorf("unexpected type %T for field dns_zone_records", values[i])
			} else if value.Valid {
				dr.dns_zone_records = new(xid.ID)
				*dr.dns_zone_records = *value.S.(*xid.ID)
			}
		}
	}
	return nil
}

// QueryZone queries the "zone" edge of the DnsRR entity.
func (dr *DnsRR) QueryZone() *DNSZoneQuery {
	return NewDnsRRClient(dr.config).QueryZone(dr)
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
	builder.WriteString("rrtype=")
	builder.WriteString(fmt.Sprintf("%v", dr.Rrtype))
	builder.WriteString(", ")
	builder.WriteString("rrcontent=")
	builder.WriteString(dr.Rrcontent)
	builder.WriteString(", ")
	builder.WriteString("class=")
	builder.WriteString(fmt.Sprintf("%v", dr.Class))
	builder.WriteString(", ")
	builder.WriteString("ttl=")
	builder.WriteString(fmt.Sprintf("%v", dr.TTL))
	builder.WriteString(", ")
	builder.WriteString("rdlength=")
	builder.WriteString(fmt.Sprintf("%v", dr.Rdlength))
	builder.WriteString(", ")
	builder.WriteString("activated=")
	builder.WriteString(fmt.Sprintf("%v", dr.Activated))
	builder.WriteByte(')')
	return builder.String()
}

// DnsRRs is a parsable slice of DnsRR.
type DnsRRs []*DnsRR
