package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/miekg/dns"
)

// DnsZone holds the schema definition for the DnsZone entity.
type DnsZone struct {
	ent.Schema
}

// Fields of the DNSZone.
func (DnsZone) Fields() []ent.Field {
	return []ent.Field{
		// SOA.Hdr.Name
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

		// SOA.Hdr.Rrtype
		field.Uint16("rrtype").
			Default(dns.TypeSOA).
			Comment("resource record type"),

		// SOA.Hdr.Class
		field.Uint16("class").
			Default(dns.ClassINET).
			Comment("class"),

		// SOA.Hdr.Ttl
		field.Uint32("ttl").
			Min(360).
			Max(2147483647).
			Default(3600).
			Comment("Time-to-live"),

		// SOA.Hdr.Rdlength
		// should we save this?
		field.Uint16("rdlength").
			Default(0).
			Comment("length of data after header"),

		// SOA.Ns
		field.String("ns").
			MinLen(3).
			MaxLen(255).
			NotEmpty().
			SchemaType(map[string]string{
				dialect.MySQL:    "varchar(255)",
				dialect.Postgres: "varchar(255)",
				dialect.SQLite:   "varchar",
			}).
			Comment("primary master name server for this zone"),

		// SOA.Mbox
		field.String("mbox").
			MinLen(3).
			MaxLen(253).
			NotEmpty().
			SchemaType(map[string]string{
				dialect.MySQL:    "varchar(255)",
				dialect.Postgres: "varchar(255)",
				dialect.SQLite:   "varchar",
			}).
			Comment("email address of the administrator responsible for this zone. (As usual, the email address is encoded as a name. The part of the email address before the @ becomes the first label of the name; the domain name after the @ becomes the rest of the name. In zone-file format, dots in labels are escaped with backslashes; thus the email address john.doe@example.com would be represented in a zone file as john.doe.example.com.)"),

		// SOA.Serial
		field.Uint32("serial").
			Comment("serial"),

		// SOA.Refresh
		field.Uint32("refresh").
			Min(360).
			Max(2147483647).
			Default(10800).
			Comment("number of seconds after which secondary name servers should query the master for the SOA record, to detect zone changes. Recommendation for small and stable zones:[4] 86400 seconds (24 hours)."),

		// SOA.Retry
		field.Uint32("retry").
			Min(360).
			Max(2147483647).
			Default(3600).
			Comment("Number of seconds after which secondary name servers should retry to request the serial number from the master if the master does not respond. It must be less than Refresh. Recommendation for small and stable zones: 7200 seconds (2 hours)."),

		// SOA.Expire
		field.Uint32("expire").
			Min(360).
			Max(2147483647).
			Default(604800).
			Comment("Number of seconds after which secondary name servers should stop answering request for this zone if the master does not respond. This value must be bigger than the sum of Refresh and Retry. Recommendation for small and stable zones: 3600000 seconds (1000 hours)."),

		// SOA.Minttl
		field.Uint32("minttl").
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
func (DnsZone) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("records", DnsRR.Type).Annotations(
			entsql.Annotation{
				OnDelete: entsql.Cascade,
			}),
	}
}

func (DnsZone) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("activated"),
	}
}

// Mixin of the DNZZone.
func (DnsZone) Mixin() []ent.Mixin {
	return []ent.Mixin{
		XidMixin{},
	}
}
