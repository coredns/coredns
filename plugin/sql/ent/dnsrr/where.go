// Code generated by ent, DO NOT EDIT.

package dnsrr

import (
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"github.com/coredns/coredns/plugin/sql/ent/predicate"
	"github.com/rs/xid"
)

// ID filters vertices based on their ID field.
func ID(id xid.ID) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldID, id))
}

// IDEQ applies the EQ predicate on the ID field.
func IDEQ(id xid.ID) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldID, id))
}

// IDNEQ applies the NEQ predicate on the ID field.
func IDNEQ(id xid.ID) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNEQ(FieldID, id))
}

// IDIn applies the In predicate on the ID field.
func IDIn(ids ...xid.ID) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldIn(FieldID, ids...))
}

// IDNotIn applies the NotIn predicate on the ID field.
func IDNotIn(ids ...xid.ID) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNotIn(FieldID, ids...))
}

// IDGT applies the GT predicate on the ID field.
func IDGT(id xid.ID) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGT(FieldID, id))
}

// IDGTE applies the GTE predicate on the ID field.
func IDGTE(id xid.ID) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGTE(FieldID, id))
}

// IDLT applies the LT predicate on the ID field.
func IDLT(id xid.ID) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLT(FieldID, id))
}

// IDLTE applies the LTE predicate on the ID field.
func IDLTE(id xid.ID) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLTE(FieldID, id))
}

// CreatedAt applies equality check predicate on the "created_at" field. It's identical to CreatedAtEQ.
func CreatedAt(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldCreatedAt, v))
}

// UpdatedAt applies equality check predicate on the "updated_at" field. It's identical to UpdatedAtEQ.
func UpdatedAt(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldUpdatedAt, v))
}

// Name applies equality check predicate on the "name" field. It's identical to NameEQ.
func Name(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldName, v))
}

// Rrtype applies equality check predicate on the "rrtype" field. It's identical to RrtypeEQ.
func Rrtype(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldRrtype, v))
}

// Rrdata applies equality check predicate on the "rrdata" field. It's identical to RrdataEQ.
func Rrdata(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldRrdata, v))
}

// Class applies equality check predicate on the "class" field. It's identical to ClassEQ.
func Class(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldClass, v))
}

// TTL applies equality check predicate on the "ttl" field. It's identical to TTLEQ.
func TTL(v uint32) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldTTL, v))
}

// Activated applies equality check predicate on the "activated" field. It's identical to ActivatedEQ.
func Activated(v bool) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldActivated, v))
}

// CreatedAtEQ applies the EQ predicate on the "created_at" field.
func CreatedAtEQ(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldCreatedAt, v))
}

// CreatedAtNEQ applies the NEQ predicate on the "created_at" field.
func CreatedAtNEQ(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNEQ(FieldCreatedAt, v))
}

// CreatedAtIn applies the In predicate on the "created_at" field.
func CreatedAtIn(vs ...time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldIn(FieldCreatedAt, vs...))
}

// CreatedAtNotIn applies the NotIn predicate on the "created_at" field.
func CreatedAtNotIn(vs ...time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNotIn(FieldCreatedAt, vs...))
}

// CreatedAtGT applies the GT predicate on the "created_at" field.
func CreatedAtGT(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGT(FieldCreatedAt, v))
}

// CreatedAtGTE applies the GTE predicate on the "created_at" field.
func CreatedAtGTE(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGTE(FieldCreatedAt, v))
}

// CreatedAtLT applies the LT predicate on the "created_at" field.
func CreatedAtLT(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLT(FieldCreatedAt, v))
}

// CreatedAtLTE applies the LTE predicate on the "created_at" field.
func CreatedAtLTE(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLTE(FieldCreatedAt, v))
}

// UpdatedAtEQ applies the EQ predicate on the "updated_at" field.
func UpdatedAtEQ(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldUpdatedAt, v))
}

// UpdatedAtNEQ applies the NEQ predicate on the "updated_at" field.
func UpdatedAtNEQ(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNEQ(FieldUpdatedAt, v))
}

// UpdatedAtIn applies the In predicate on the "updated_at" field.
func UpdatedAtIn(vs ...time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldIn(FieldUpdatedAt, vs...))
}

// UpdatedAtNotIn applies the NotIn predicate on the "updated_at" field.
func UpdatedAtNotIn(vs ...time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNotIn(FieldUpdatedAt, vs...))
}

// UpdatedAtGT applies the GT predicate on the "updated_at" field.
func UpdatedAtGT(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGT(FieldUpdatedAt, v))
}

// UpdatedAtGTE applies the GTE predicate on the "updated_at" field.
func UpdatedAtGTE(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGTE(FieldUpdatedAt, v))
}

// UpdatedAtLT applies the LT predicate on the "updated_at" field.
func UpdatedAtLT(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLT(FieldUpdatedAt, v))
}

// UpdatedAtLTE applies the LTE predicate on the "updated_at" field.
func UpdatedAtLTE(v time.Time) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLTE(FieldUpdatedAt, v))
}

// NameEQ applies the EQ predicate on the "name" field.
func NameEQ(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldName, v))
}

// NameNEQ applies the NEQ predicate on the "name" field.
func NameNEQ(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNEQ(FieldName, v))
}

// NameIn applies the In predicate on the "name" field.
func NameIn(vs ...string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldIn(FieldName, vs...))
}

// NameNotIn applies the NotIn predicate on the "name" field.
func NameNotIn(vs ...string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNotIn(FieldName, vs...))
}

// NameGT applies the GT predicate on the "name" field.
func NameGT(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGT(FieldName, v))
}

// NameGTE applies the GTE predicate on the "name" field.
func NameGTE(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGTE(FieldName, v))
}

// NameLT applies the LT predicate on the "name" field.
func NameLT(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLT(FieldName, v))
}

// NameLTE applies the LTE predicate on the "name" field.
func NameLTE(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLTE(FieldName, v))
}

// NameContains applies the Contains predicate on the "name" field.
func NameContains(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldContains(FieldName, v))
}

// NameHasPrefix applies the HasPrefix predicate on the "name" field.
func NameHasPrefix(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldHasPrefix(FieldName, v))
}

// NameHasSuffix applies the HasSuffix predicate on the "name" field.
func NameHasSuffix(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldHasSuffix(FieldName, v))
}

// NameEqualFold applies the EqualFold predicate on the "name" field.
func NameEqualFold(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEqualFold(FieldName, v))
}

// NameContainsFold applies the ContainsFold predicate on the "name" field.
func NameContainsFold(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldContainsFold(FieldName, v))
}

// RrtypeEQ applies the EQ predicate on the "rrtype" field.
func RrtypeEQ(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldRrtype, v))
}

// RrtypeNEQ applies the NEQ predicate on the "rrtype" field.
func RrtypeNEQ(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNEQ(FieldRrtype, v))
}

// RrtypeIn applies the In predicate on the "rrtype" field.
func RrtypeIn(vs ...uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldIn(FieldRrtype, vs...))
}

// RrtypeNotIn applies the NotIn predicate on the "rrtype" field.
func RrtypeNotIn(vs ...uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNotIn(FieldRrtype, vs...))
}

// RrtypeGT applies the GT predicate on the "rrtype" field.
func RrtypeGT(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGT(FieldRrtype, v))
}

// RrtypeGTE applies the GTE predicate on the "rrtype" field.
func RrtypeGTE(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGTE(FieldRrtype, v))
}

// RrtypeLT applies the LT predicate on the "rrtype" field.
func RrtypeLT(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLT(FieldRrtype, v))
}

// RrtypeLTE applies the LTE predicate on the "rrtype" field.
func RrtypeLTE(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLTE(FieldRrtype, v))
}

// RrdataEQ applies the EQ predicate on the "rrdata" field.
func RrdataEQ(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldRrdata, v))
}

// RrdataNEQ applies the NEQ predicate on the "rrdata" field.
func RrdataNEQ(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNEQ(FieldRrdata, v))
}

// RrdataIn applies the In predicate on the "rrdata" field.
func RrdataIn(vs ...string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldIn(FieldRrdata, vs...))
}

// RrdataNotIn applies the NotIn predicate on the "rrdata" field.
func RrdataNotIn(vs ...string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNotIn(FieldRrdata, vs...))
}

// RrdataGT applies the GT predicate on the "rrdata" field.
func RrdataGT(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGT(FieldRrdata, v))
}

// RrdataGTE applies the GTE predicate on the "rrdata" field.
func RrdataGTE(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGTE(FieldRrdata, v))
}

// RrdataLT applies the LT predicate on the "rrdata" field.
func RrdataLT(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLT(FieldRrdata, v))
}

// RrdataLTE applies the LTE predicate on the "rrdata" field.
func RrdataLTE(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLTE(FieldRrdata, v))
}

// RrdataContains applies the Contains predicate on the "rrdata" field.
func RrdataContains(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldContains(FieldRrdata, v))
}

// RrdataHasPrefix applies the HasPrefix predicate on the "rrdata" field.
func RrdataHasPrefix(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldHasPrefix(FieldRrdata, v))
}

// RrdataHasSuffix applies the HasSuffix predicate on the "rrdata" field.
func RrdataHasSuffix(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldHasSuffix(FieldRrdata, v))
}

// RrdataEqualFold applies the EqualFold predicate on the "rrdata" field.
func RrdataEqualFold(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEqualFold(FieldRrdata, v))
}

// RrdataContainsFold applies the ContainsFold predicate on the "rrdata" field.
func RrdataContainsFold(v string) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldContainsFold(FieldRrdata, v))
}

// ClassEQ applies the EQ predicate on the "class" field.
func ClassEQ(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldClass, v))
}

// ClassNEQ applies the NEQ predicate on the "class" field.
func ClassNEQ(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNEQ(FieldClass, v))
}

// ClassIn applies the In predicate on the "class" field.
func ClassIn(vs ...uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldIn(FieldClass, vs...))
}

// ClassNotIn applies the NotIn predicate on the "class" field.
func ClassNotIn(vs ...uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNotIn(FieldClass, vs...))
}

// ClassGT applies the GT predicate on the "class" field.
func ClassGT(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGT(FieldClass, v))
}

// ClassGTE applies the GTE predicate on the "class" field.
func ClassGTE(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGTE(FieldClass, v))
}

// ClassLT applies the LT predicate on the "class" field.
func ClassLT(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLT(FieldClass, v))
}

// ClassLTE applies the LTE predicate on the "class" field.
func ClassLTE(v uint16) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLTE(FieldClass, v))
}

// TTLEQ applies the EQ predicate on the "ttl" field.
func TTLEQ(v uint32) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldTTL, v))
}

// TTLNEQ applies the NEQ predicate on the "ttl" field.
func TTLNEQ(v uint32) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNEQ(FieldTTL, v))
}

// TTLIn applies the In predicate on the "ttl" field.
func TTLIn(vs ...uint32) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldIn(FieldTTL, vs...))
}

// TTLNotIn applies the NotIn predicate on the "ttl" field.
func TTLNotIn(vs ...uint32) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNotIn(FieldTTL, vs...))
}

// TTLGT applies the GT predicate on the "ttl" field.
func TTLGT(v uint32) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGT(FieldTTL, v))
}

// TTLGTE applies the GTE predicate on the "ttl" field.
func TTLGTE(v uint32) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldGTE(FieldTTL, v))
}

// TTLLT applies the LT predicate on the "ttl" field.
func TTLLT(v uint32) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLT(FieldTTL, v))
}

// TTLLTE applies the LTE predicate on the "ttl" field.
func TTLLTE(v uint32) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldLTE(FieldTTL, v))
}

// ActivatedEQ applies the EQ predicate on the "activated" field.
func ActivatedEQ(v bool) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldEQ(FieldActivated, v))
}

// ActivatedNEQ applies the NEQ predicate on the "activated" field.
func ActivatedNEQ(v bool) predicate.DnsRR {
	return predicate.DnsRR(sql.FieldNEQ(FieldActivated, v))
}

// HasZone applies the HasEdge predicate on the "zone" edge.
func HasZone() predicate.DnsRR {
	return predicate.DnsRR(func(s *sql.Selector) {
		step := sqlgraph.NewStep(
			sqlgraph.From(Table, FieldID),
			sqlgraph.Edge(sqlgraph.M2O, true, ZoneTable, ZoneColumn),
		)
		sqlgraph.HasNeighbors(s, step)
	})
}

// HasZoneWith applies the HasEdge predicate on the "zone" edge with a given conditions (other predicates).
func HasZoneWith(preds ...predicate.DnsZone) predicate.DnsRR {
	return predicate.DnsRR(func(s *sql.Selector) {
		step := newZoneStep()
		sqlgraph.HasNeighborsWith(s, step, func(s *sql.Selector) {
			for _, p := range preds {
				p(s)
			}
		})
	})
}

// And groups predicates with the AND operator between them.
func And(predicates ...predicate.DnsRR) predicate.DnsRR {
	return predicate.DnsRR(func(s *sql.Selector) {
		s1 := s.Clone().SetP(nil)
		for _, p := range predicates {
			p(s1)
		}
		s.Where(s1.P())
	})
}

// Or groups predicates with the OR operator between them.
func Or(predicates ...predicate.DnsRR) predicate.DnsRR {
	return predicate.DnsRR(func(s *sql.Selector) {
		s1 := s.Clone().SetP(nil)
		for i, p := range predicates {
			if i > 0 {
				s1.Or()
			}
			p(s1)
		}
		s.Where(s1.P())
	})
}

// Not applies the not operator on the given predicate.
func Not(p predicate.DnsRR) predicate.DnsRR {
	return predicate.DnsRR(func(s *sql.Selector) {
		p(s.Not())
	})
}
