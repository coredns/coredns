package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
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
		field.Int32("ttl").
			Min(360).
			Max(2147483647).
			Default(3600).Comment("Time-to-live"),
		field.Bool("activated").
			Default(true).
			Comment("only activated resource records will be served"),
	}
}

// Edges of the DnsRR.
func (DnsRR) Edges() []ent.Edge {
	return nil
}

// Mixin of the DNSRecord.
func (DnsRR) Mixin() []ent.Mixin {
	return []ent.Mixin{
		XidMixin{},
	}
}
