// Code generated by ent, DO NOT EDIT.

package migrate

import (
	"entgo.io/ent/dialect/sql/schema"
	"entgo.io/ent/schema/field"
)

var (
	// DNSZonesColumns holds the columns for the "dns_zones" table.
	DNSZonesColumns = []*schema.Column{
		{Name: "id", Type: field.TypeString},
		{Name: "created_at", Type: field.TypeTime, SchemaType: map[string]string{"mysql": "datetime(6)"}},
		{Name: "updated_at", Type: field.TypeTime, SchemaType: map[string]string{"mysql": "datetime(6)"}},
		{Name: "name", Type: field.TypeString, Unique: true, Size: 255, SchemaType: map[string]string{"mysql": "varchar(255)", "postgres": "varchar(255)", "sqlite3": "varchar"}},
		{Name: "mname", Type: field.TypeString, Unique: true, Size: 255, SchemaType: map[string]string{"mysql": "varchar(253)", "postgres": "varchar(253)", "sqlite3": "varchar(253)"}},
		{Name: "rname", Type: field.TypeString, Size: 253, SchemaType: map[string]string{"mysql": "varchar(253)", "postgres": "varchar(253)", "sqlite3": "varchar(253)"}},
		{Name: "ttl", Type: field.TypeInt32, Default: 3600},
		{Name: "refresh", Type: field.TypeInt32, Default: 10800},
		{Name: "retry", Type: field.TypeInt32, Default: 3600},
		{Name: "expire", Type: field.TypeInt32, Default: 604800},
		{Name: "minimum", Type: field.TypeInt32, Default: 3600},
		{Name: "activated", Type: field.TypeBool, Default: true},
	}
	// DNSZonesTable holds the schema information for the "dns_zones" table.
	DNSZonesTable = &schema.Table{
		Name:       "dns_zones",
		Columns:    DNSZonesColumns,
		PrimaryKey: []*schema.Column{DNSZonesColumns[0]},
	}
	// DNSRrsColumns holds the columns for the "dns_rrs" table.
	DNSRrsColumns = []*schema.Column{
		{Name: "id", Type: field.TypeString},
		{Name: "created_at", Type: field.TypeTime, SchemaType: map[string]string{"mysql": "datetime(6)"}},
		{Name: "updated_at", Type: field.TypeTime, SchemaType: map[string]string{"mysql": "datetime(6)"}},
		{Name: "name", Type: field.TypeString, Size: 255, SchemaType: map[string]string{"mysql": "varchar(255)", "postgres": "varchar(255)", "sqlite3": "varchar"}},
		{Name: "ttl", Type: field.TypeInt32, Default: 3600},
		{Name: "activated", Type: field.TypeBool, Default: true},
		{Name: "dns_zone_records", Type: field.TypeString},
	}
	// DNSRrsTable holds the schema information for the "dns_rrs" table.
	DNSRrsTable = &schema.Table{
		Name:       "dns_rrs",
		Columns:    DNSRrsColumns,
		PrimaryKey: []*schema.Column{DNSRrsColumns[0]},
		ForeignKeys: []*schema.ForeignKey{
			{
				Symbol:     "dns_rrs_dns_zones_records",
				Columns:    []*schema.Column{DNSRrsColumns[6]},
				RefColumns: []*schema.Column{DNSZonesColumns[0]},
				OnDelete:   schema.Cascade,
			},
		},
	}
	// Tables holds all the tables in the schema.
	Tables = []*schema.Table{
		DNSZonesTable,
		DNSRrsTable,
	}
)

func init() {
	DNSRrsTable.ForeignKeys[0].RefTable = DNSZonesTable
}
