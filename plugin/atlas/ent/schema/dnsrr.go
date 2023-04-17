package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/miekg/dns"
)

// DnsRR holds the schema definition for the DnsRR entity.
type DnsRR struct {
	ent.Schema
}

// Fields of the DnsRR.
func (DnsRR) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MinLen(1).
			MaxLen(255).
			NotEmpty().
			Immutable().
			SchemaType(map[string]string{
				dialect.MySQL:    "varchar(255)",
				dialect.Postgres: "varchar(255)",
				dialect.SQLite:   "varchar", // check: SQLite has no varchar length
			}),

		// dns.<type>.Hdr.Rrtype
		field.Uint16("rrtype").
			Comment("resource record type"),

		field.Text("rrcontent").
			Comment("resource record content"),

		// SOA.Hdr.Class
		field.Uint16("class").
			Default(dns.ClassINET).
			Comment("class"),

		field.Uint32("ttl").
			Min(360).
			Max(2147483647).
			Default(3600).Comment("Time-to-live"),

		// SOA.Hdr.Rdlength
		// should we save this?
		field.Uint16("rdlength").
			Comment("length of data after header"),

		field.Bool("activated").
			Default(true).
			Comment("only activated resource records will be served"),
	}
}

// Edges of the DnsRR.
func (DnsRR) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("zone", DnsZone.Type).
			Ref("records").
			Unique().
			Required(),
	}
}

func (DnsRR) Indexes() []ent.Index {
	return []ent.Index{
		// non-unique index.
		index.Fields("name", "rrtype"),
		index.Fields("activated"),
	}
}

// Mixin of the DNSRecord.
func (DnsRR) Mixin() []ent.Mixin {
	return []ent.Mixin{
		XidMixin{},
	}
}
