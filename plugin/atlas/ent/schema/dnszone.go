package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// DNSZone holds the schema definition for the DNSZone entity.
type DNSZone struct {
	ent.Schema
}

// Fields of the DNSZone.
func (DNSZone) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MinLen(3).
			MaxLen(255).
			NotEmpty().
			Unique().
			Immutable().
			SchemaType(map[string]string{
				dialect.MySQL:    "varchar(255)",
				dialect.Postgres: "varchar(255)",
				dialect.SQLite:   "varchar",
			}).
			Comment("dns zone name must be end with a dot '.' ex: 'example.com.'"),
		field.String("mname").
			MinLen(3).
			MaxLen(255).
			NotEmpty().
			Unique().
			Immutable().
			SchemaType(map[string]string{
				dialect.MySQL:    "varchar(253)",
				dialect.Postgres: "varchar(253)",
				dialect.SQLite:   "varchar(253)",
			}).
			Comment("primary master name server for this zone"),

		// rname: webmaster@domain.tld
		field.String("rname").
			MinLen(3).
			MaxLen(253).
			NotEmpty().
			SchemaType(map[string]string{
				dialect.MySQL:    "varchar(253)",
				dialect.Postgres: "varchar(253)",
				dialect.SQLite:   "varchar(253)",
			}).
			Comment("email address of the administrator responsible for this zone. (As usual, the email address is encoded as a name. The part of the email address before the @ becomes the first label of the name; the domain name after the @ becomes the rest of the name. In zone-file format, dots in labels are escaped with backslashes; thus the email address john.doe@example.com would be represented in a zone file as john.doe.example.com.)"),

		field.Int32("ttl").
			Min(360).
			Max(2147483647).
			Default(3600).
			Comment("Time-to-live"),

		field.Int32("refresh").
			Min(360).
			Max(2147483647).
			Default(10800).
			Comment("number of seconds after which secondary name servers should query the master for the SOA record, to detect zone changes. Recommendation for small and stable zones:[4] 86400 seconds (24 hours)."),

		field.Int32("retry").
			Min(360).
			Max(2147483647).
			Default(3600).
			Comment("Number of seconds after which secondary name servers should retry to request the serial number from the master if the master does not respond. It must be less than Refresh. Recommendation for small and stable zones: 7200 seconds (2 hours)."),

		field.Int32("expire").
			Min(360).
			Max(2147483647).
			Default(604800).
			Comment("Number of seconds after which secondary name servers should stop answering request for this zone if the master does not respond. This value must be bigger than the sum of Refresh and Retry. Recommendation for small and stable zones: 3600000 seconds (1000 hours)."),

		field.Int32("minimum").
			Min(360).
			Max(2147483647).
			Default(3600).
			Comment("The unsigned 32 bit minimum TTL field that should be exported with any RR from this zone."),

		field.Bool("activated").
			Default(true).
			Comment("only activated zones will be served"),
	}
}

// Edges of the DNSZone.
func (DNSZone) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("records", DnsRR.Type).Annotations(
			entsql.Annotation{
				OnDelete: entsql.Cascade,
			}),
	}
}

// Mixin of the DNZZone.
func (DNSZone) Mixin() []ent.Mixin {
	return []ent.Mixin{
		XidMixin{},
	}
}
