// Code generated by ent, DO NOT EDIT.

package ent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/schema/field"
	"github.com/coredns/coredns/plugin/sql/ent/dnsrr"
	"github.com/coredns/coredns/plugin/sql/ent/dnszone"
	"github.com/coredns/coredns/plugin/sql/ent/predicate"
	"github.com/rs/xid"
)

// DnsZoneUpdate is the builder for updating DnsZone entities.
type DnsZoneUpdate struct {
	config
	hooks    []Hook
	mutation *DnsZoneMutation
}

// Where appends a list predicates to the DnsZoneUpdate builder.
func (dzu *DnsZoneUpdate) Where(ps ...predicate.DnsZone) *DnsZoneUpdate {
	dzu.mutation.Where(ps...)
	return dzu
}

// SetUpdatedAt sets the "updated_at" field.
func (dzu *DnsZoneUpdate) SetUpdatedAt(t time.Time) *DnsZoneUpdate {
	dzu.mutation.SetUpdatedAt(t)
	return dzu
}

// SetRrtype sets the "rrtype" field.
func (dzu *DnsZoneUpdate) SetRrtype(u uint16) *DnsZoneUpdate {
	dzu.mutation.ResetRrtype()
	dzu.mutation.SetRrtype(u)
	return dzu
}

// SetNillableRrtype sets the "rrtype" field if the given value is not nil.
func (dzu *DnsZoneUpdate) SetNillableRrtype(u *uint16) *DnsZoneUpdate {
	if u != nil {
		dzu.SetRrtype(*u)
	}
	return dzu
}

// AddRrtype adds u to the "rrtype" field.
func (dzu *DnsZoneUpdate) AddRrtype(u int16) *DnsZoneUpdate {
	dzu.mutation.AddRrtype(u)
	return dzu
}

// SetClass sets the "class" field.
func (dzu *DnsZoneUpdate) SetClass(u uint16) *DnsZoneUpdate {
	dzu.mutation.ResetClass()
	dzu.mutation.SetClass(u)
	return dzu
}

// SetNillableClass sets the "class" field if the given value is not nil.
func (dzu *DnsZoneUpdate) SetNillableClass(u *uint16) *DnsZoneUpdate {
	if u != nil {
		dzu.SetClass(*u)
	}
	return dzu
}

// AddClass adds u to the "class" field.
func (dzu *DnsZoneUpdate) AddClass(u int16) *DnsZoneUpdate {
	dzu.mutation.AddClass(u)
	return dzu
}

// SetTTL sets the "ttl" field.
func (dzu *DnsZoneUpdate) SetTTL(u uint32) *DnsZoneUpdate {
	dzu.mutation.ResetTTL()
	dzu.mutation.SetTTL(u)
	return dzu
}

// SetNillableTTL sets the "ttl" field if the given value is not nil.
func (dzu *DnsZoneUpdate) SetNillableTTL(u *uint32) *DnsZoneUpdate {
	if u != nil {
		dzu.SetTTL(*u)
	}
	return dzu
}

// AddTTL adds u to the "ttl" field.
func (dzu *DnsZoneUpdate) AddTTL(u int32) *DnsZoneUpdate {
	dzu.mutation.AddTTL(u)
	return dzu
}

// SetNs sets the "ns" field.
func (dzu *DnsZoneUpdate) SetNs(s string) *DnsZoneUpdate {
	dzu.mutation.SetNs(s)
	return dzu
}

// SetMbox sets the "mbox" field.
func (dzu *DnsZoneUpdate) SetMbox(s string) *DnsZoneUpdate {
	dzu.mutation.SetMbox(s)
	return dzu
}

// SetSerial sets the "serial" field.
func (dzu *DnsZoneUpdate) SetSerial(u uint32) *DnsZoneUpdate {
	dzu.mutation.ResetSerial()
	dzu.mutation.SetSerial(u)
	return dzu
}

// AddSerial adds u to the "serial" field.
func (dzu *DnsZoneUpdate) AddSerial(u int32) *DnsZoneUpdate {
	dzu.mutation.AddSerial(u)
	return dzu
}

// SetRefresh sets the "refresh" field.
func (dzu *DnsZoneUpdate) SetRefresh(u uint32) *DnsZoneUpdate {
	dzu.mutation.ResetRefresh()
	dzu.mutation.SetRefresh(u)
	return dzu
}

// SetNillableRefresh sets the "refresh" field if the given value is not nil.
func (dzu *DnsZoneUpdate) SetNillableRefresh(u *uint32) *DnsZoneUpdate {
	if u != nil {
		dzu.SetRefresh(*u)
	}
	return dzu
}

// AddRefresh adds u to the "refresh" field.
func (dzu *DnsZoneUpdate) AddRefresh(u int32) *DnsZoneUpdate {
	dzu.mutation.AddRefresh(u)
	return dzu
}

// SetRetry sets the "retry" field.
func (dzu *DnsZoneUpdate) SetRetry(u uint32) *DnsZoneUpdate {
	dzu.mutation.ResetRetry()
	dzu.mutation.SetRetry(u)
	return dzu
}

// SetNillableRetry sets the "retry" field if the given value is not nil.
func (dzu *DnsZoneUpdate) SetNillableRetry(u *uint32) *DnsZoneUpdate {
	if u != nil {
		dzu.SetRetry(*u)
	}
	return dzu
}

// AddRetry adds u to the "retry" field.
func (dzu *DnsZoneUpdate) AddRetry(u int32) *DnsZoneUpdate {
	dzu.mutation.AddRetry(u)
	return dzu
}

// SetExpire sets the "expire" field.
func (dzu *DnsZoneUpdate) SetExpire(u uint32) *DnsZoneUpdate {
	dzu.mutation.ResetExpire()
	dzu.mutation.SetExpire(u)
	return dzu
}

// SetNillableExpire sets the "expire" field if the given value is not nil.
func (dzu *DnsZoneUpdate) SetNillableExpire(u *uint32) *DnsZoneUpdate {
	if u != nil {
		dzu.SetExpire(*u)
	}
	return dzu
}

// AddExpire adds u to the "expire" field.
func (dzu *DnsZoneUpdate) AddExpire(u int32) *DnsZoneUpdate {
	dzu.mutation.AddExpire(u)
	return dzu
}

// SetMinttl sets the "minttl" field.
func (dzu *DnsZoneUpdate) SetMinttl(u uint32) *DnsZoneUpdate {
	dzu.mutation.ResetMinttl()
	dzu.mutation.SetMinttl(u)
	return dzu
}

// SetNillableMinttl sets the "minttl" field if the given value is not nil.
func (dzu *DnsZoneUpdate) SetNillableMinttl(u *uint32) *DnsZoneUpdate {
	if u != nil {
		dzu.SetMinttl(*u)
	}
	return dzu
}

// AddMinttl adds u to the "minttl" field.
func (dzu *DnsZoneUpdate) AddMinttl(u int32) *DnsZoneUpdate {
	dzu.mutation.AddMinttl(u)
	return dzu
}

// SetActivated sets the "activated" field.
func (dzu *DnsZoneUpdate) SetActivated(b bool) *DnsZoneUpdate {
	dzu.mutation.SetActivated(b)
	return dzu
}

// SetNillableActivated sets the "activated" field if the given value is not nil.
func (dzu *DnsZoneUpdate) SetNillableActivated(b *bool) *DnsZoneUpdate {
	if b != nil {
		dzu.SetActivated(*b)
	}
	return dzu
}

// AddRecordIDs adds the "records" edge to the DnsRR entity by IDs.
func (dzu *DnsZoneUpdate) AddRecordIDs(ids ...xid.ID) *DnsZoneUpdate {
	dzu.mutation.AddRecordIDs(ids...)
	return dzu
}

// AddRecords adds the "records" edges to the DnsRR entity.
func (dzu *DnsZoneUpdate) AddRecords(d ...*DnsRR) *DnsZoneUpdate {
	ids := make([]xid.ID, len(d))
	for i := range d {
		ids[i] = d[i].ID
	}
	return dzu.AddRecordIDs(ids...)
}

// Mutation returns the DnsZoneMutation object of the builder.
func (dzu *DnsZoneUpdate) Mutation() *DnsZoneMutation {
	return dzu.mutation
}

// ClearRecords clears all "records" edges to the DnsRR entity.
func (dzu *DnsZoneUpdate) ClearRecords() *DnsZoneUpdate {
	dzu.mutation.ClearRecords()
	return dzu
}

// RemoveRecordIDs removes the "records" edge to DnsRR entities by IDs.
func (dzu *DnsZoneUpdate) RemoveRecordIDs(ids ...xid.ID) *DnsZoneUpdate {
	dzu.mutation.RemoveRecordIDs(ids...)
	return dzu
}

// RemoveRecords removes "records" edges to DnsRR entities.
func (dzu *DnsZoneUpdate) RemoveRecords(d ...*DnsRR) *DnsZoneUpdate {
	ids := make([]xid.ID, len(d))
	for i := range d {
		ids[i] = d[i].ID
	}
	return dzu.RemoveRecordIDs(ids...)
}

// Save executes the query and returns the number of nodes affected by the update operation.
func (dzu *DnsZoneUpdate) Save(ctx context.Context) (int, error) {
	dzu.defaults()
	return withHooks[int, DnsZoneMutation](ctx, dzu.sqlSave, dzu.mutation, dzu.hooks)
}

// SaveX is like Save, but panics if an error occurs.
func (dzu *DnsZoneUpdate) SaveX(ctx context.Context) int {
	affected, err := dzu.Save(ctx)
	if err != nil {
		panic(err)
	}
	return affected
}

// Exec executes the query.
func (dzu *DnsZoneUpdate) Exec(ctx context.Context) error {
	_, err := dzu.Save(ctx)
	return err
}

// ExecX is like Exec, but panics if an error occurs.
func (dzu *DnsZoneUpdate) ExecX(ctx context.Context) {
	if err := dzu.Exec(ctx); err != nil {
		panic(err)
	}
}

// defaults sets the default values of the builder before save.
func (dzu *DnsZoneUpdate) defaults() {
	if _, ok := dzu.mutation.UpdatedAt(); !ok {
		v := dnszone.UpdateDefaultUpdatedAt()
		dzu.mutation.SetUpdatedAt(v)
	}
}

// check runs all checks and user-defined validators on the builder.
func (dzu *DnsZoneUpdate) check() error {
	if v, ok := dzu.mutation.Rrtype(); ok {
		if err := dnszone.RrtypeValidator(v); err != nil {
			return &ValidationError{Name: "rrtype", err: fmt.Errorf(`ent: validator failed for field "DnsZone.rrtype": %w`, err)}
		}
	}
	if v, ok := dzu.mutation.Class(); ok {
		if err := dnszone.ClassValidator(v); err != nil {
			return &ValidationError{Name: "class", err: fmt.Errorf(`ent: validator failed for field "DnsZone.class": %w`, err)}
		}
	}
	if v, ok := dzu.mutation.TTL(); ok {
		if err := dnszone.TTLValidator(v); err != nil {
			return &ValidationError{Name: "ttl", err: fmt.Errorf(`ent: validator failed for field "DnsZone.ttl": %w`, err)}
		}
	}
	if v, ok := dzu.mutation.Ns(); ok {
		if err := dnszone.NsValidator(v); err != nil {
			return &ValidationError{Name: "ns", err: fmt.Errorf(`ent: validator failed for field "DnsZone.ns": %w`, err)}
		}
	}
	if v, ok := dzu.mutation.Mbox(); ok {
		if err := dnszone.MboxValidator(v); err != nil {
			return &ValidationError{Name: "mbox", err: fmt.Errorf(`ent: validator failed for field "DnsZone.mbox": %w`, err)}
		}
	}
	if v, ok := dzu.mutation.Serial(); ok {
		if err := dnszone.SerialValidator(v); err != nil {
			return &ValidationError{Name: "serial", err: fmt.Errorf(`ent: validator failed for field "DnsZone.serial": %w`, err)}
		}
	}
	if v, ok := dzu.mutation.Refresh(); ok {
		if err := dnszone.RefreshValidator(v); err != nil {
			return &ValidationError{Name: "refresh", err: fmt.Errorf(`ent: validator failed for field "DnsZone.refresh": %w`, err)}
		}
	}
	if v, ok := dzu.mutation.Retry(); ok {
		if err := dnszone.RetryValidator(v); err != nil {
			return &ValidationError{Name: "retry", err: fmt.Errorf(`ent: validator failed for field "DnsZone.retry": %w`, err)}
		}
	}
	if v, ok := dzu.mutation.Expire(); ok {
		if err := dnszone.ExpireValidator(v); err != nil {
			return &ValidationError{Name: "expire", err: fmt.Errorf(`ent: validator failed for field "DnsZone.expire": %w`, err)}
		}
	}
	if v, ok := dzu.mutation.Minttl(); ok {
		if err := dnszone.MinttlValidator(v); err != nil {
			return &ValidationError{Name: "minttl", err: fmt.Errorf(`ent: validator failed for field "DnsZone.minttl": %w`, err)}
		}
	}
	return nil
}

func (dzu *DnsZoneUpdate) sqlSave(ctx context.Context) (n int, err error) {
	if err := dzu.check(); err != nil {
		return n, err
	}
	_spec := sqlgraph.NewUpdateSpec(dnszone.Table, dnszone.Columns, sqlgraph.NewFieldSpec(dnszone.FieldID, field.TypeString))
	if ps := dzu.mutation.predicates; len(ps) > 0 {
		_spec.Predicate = func(selector *sql.Selector) {
			for i := range ps {
				ps[i](selector)
			}
		}
	}
	if value, ok := dzu.mutation.UpdatedAt(); ok {
		_spec.SetField(dnszone.FieldUpdatedAt, field.TypeTime, value)
	}
	if value, ok := dzu.mutation.Rrtype(); ok {
		_spec.SetField(dnszone.FieldRrtype, field.TypeUint16, value)
	}
	if value, ok := dzu.mutation.AddedRrtype(); ok {
		_spec.AddField(dnszone.FieldRrtype, field.TypeUint16, value)
	}
	if value, ok := dzu.mutation.Class(); ok {
		_spec.SetField(dnszone.FieldClass, field.TypeUint16, value)
	}
	if value, ok := dzu.mutation.AddedClass(); ok {
		_spec.AddField(dnszone.FieldClass, field.TypeUint16, value)
	}
	if value, ok := dzu.mutation.TTL(); ok {
		_spec.SetField(dnszone.FieldTTL, field.TypeUint32, value)
	}
	if value, ok := dzu.mutation.AddedTTL(); ok {
		_spec.AddField(dnszone.FieldTTL, field.TypeUint32, value)
	}
	if value, ok := dzu.mutation.Ns(); ok {
		_spec.SetField(dnszone.FieldNs, field.TypeString, value)
	}
	if value, ok := dzu.mutation.Mbox(); ok {
		_spec.SetField(dnszone.FieldMbox, field.TypeString, value)
	}
	if value, ok := dzu.mutation.Serial(); ok {
		_spec.SetField(dnszone.FieldSerial, field.TypeUint32, value)
	}
	if value, ok := dzu.mutation.AddedSerial(); ok {
		_spec.AddField(dnszone.FieldSerial, field.TypeUint32, value)
	}
	if value, ok := dzu.mutation.Refresh(); ok {
		_spec.SetField(dnszone.FieldRefresh, field.TypeUint32, value)
	}
	if value, ok := dzu.mutation.AddedRefresh(); ok {
		_spec.AddField(dnszone.FieldRefresh, field.TypeUint32, value)
	}
	if value, ok := dzu.mutation.Retry(); ok {
		_spec.SetField(dnszone.FieldRetry, field.TypeUint32, value)
	}
	if value, ok := dzu.mutation.AddedRetry(); ok {
		_spec.AddField(dnszone.FieldRetry, field.TypeUint32, value)
	}
	if value, ok := dzu.mutation.Expire(); ok {
		_spec.SetField(dnszone.FieldExpire, field.TypeUint32, value)
	}
	if value, ok := dzu.mutation.AddedExpire(); ok {
		_spec.AddField(dnszone.FieldExpire, field.TypeUint32, value)
	}
	if value, ok := dzu.mutation.Minttl(); ok {
		_spec.SetField(dnszone.FieldMinttl, field.TypeUint32, value)
	}
	if value, ok := dzu.mutation.AddedMinttl(); ok {
		_spec.AddField(dnszone.FieldMinttl, field.TypeUint32, value)
	}
	if value, ok := dzu.mutation.Activated(); ok {
		_spec.SetField(dnszone.FieldActivated, field.TypeBool, value)
	}
	if dzu.mutation.RecordsCleared() {
		edge := &sqlgraph.EdgeSpec{
			Rel:     sqlgraph.O2M,
			Inverse: false,
			Table:   dnszone.RecordsTable,
			Columns: []string{dnszone.RecordsColumn},
			Bidi:    false,
			Target: &sqlgraph.EdgeTarget{
				IDSpec: sqlgraph.NewFieldSpec(dnsrr.FieldID, field.TypeString),
			},
		}
		_spec.Edges.Clear = append(_spec.Edges.Clear, edge)
	}
	if nodes := dzu.mutation.RemovedRecordsIDs(); len(nodes) > 0 && !dzu.mutation.RecordsCleared() {
		edge := &sqlgraph.EdgeSpec{
			Rel:     sqlgraph.O2M,
			Inverse: false,
			Table:   dnszone.RecordsTable,
			Columns: []string{dnszone.RecordsColumn},
			Bidi:    false,
			Target: &sqlgraph.EdgeTarget{
				IDSpec: sqlgraph.NewFieldSpec(dnsrr.FieldID, field.TypeString),
			},
		}
		for _, k := range nodes {
			edge.Target.Nodes = append(edge.Target.Nodes, k)
		}
		_spec.Edges.Clear = append(_spec.Edges.Clear, edge)
	}
	if nodes := dzu.mutation.RecordsIDs(); len(nodes) > 0 {
		edge := &sqlgraph.EdgeSpec{
			Rel:     sqlgraph.O2M,
			Inverse: false,
			Table:   dnszone.RecordsTable,
			Columns: []string{dnszone.RecordsColumn},
			Bidi:    false,
			Target: &sqlgraph.EdgeTarget{
				IDSpec: sqlgraph.NewFieldSpec(dnsrr.FieldID, field.TypeString),
			},
		}
		for _, k := range nodes {
			edge.Target.Nodes = append(edge.Target.Nodes, k)
		}
		_spec.Edges.Add = append(_spec.Edges.Add, edge)
	}
	if n, err = sqlgraph.UpdateNodes(ctx, dzu.driver, _spec); err != nil {
		if _, ok := err.(*sqlgraph.NotFoundError); ok {
			err = &NotFoundError{dnszone.Label}
		} else if sqlgraph.IsConstraintError(err) {
			err = &ConstraintError{msg: err.Error(), wrap: err}
		}
		return 0, err
	}
	dzu.mutation.done = true
	return n, nil
}

// DnsZoneUpdateOne is the builder for updating a single DnsZone entity.
type DnsZoneUpdateOne struct {
	config
	fields   []string
	hooks    []Hook
	mutation *DnsZoneMutation
}

// SetUpdatedAt sets the "updated_at" field.
func (dzuo *DnsZoneUpdateOne) SetUpdatedAt(t time.Time) *DnsZoneUpdateOne {
	dzuo.mutation.SetUpdatedAt(t)
	return dzuo
}

// SetRrtype sets the "rrtype" field.
func (dzuo *DnsZoneUpdateOne) SetRrtype(u uint16) *DnsZoneUpdateOne {
	dzuo.mutation.ResetRrtype()
	dzuo.mutation.SetRrtype(u)
	return dzuo
}

// SetNillableRrtype sets the "rrtype" field if the given value is not nil.
func (dzuo *DnsZoneUpdateOne) SetNillableRrtype(u *uint16) *DnsZoneUpdateOne {
	if u != nil {
		dzuo.SetRrtype(*u)
	}
	return dzuo
}

// AddRrtype adds u to the "rrtype" field.
func (dzuo *DnsZoneUpdateOne) AddRrtype(u int16) *DnsZoneUpdateOne {
	dzuo.mutation.AddRrtype(u)
	return dzuo
}

// SetClass sets the "class" field.
func (dzuo *DnsZoneUpdateOne) SetClass(u uint16) *DnsZoneUpdateOne {
	dzuo.mutation.ResetClass()
	dzuo.mutation.SetClass(u)
	return dzuo
}

// SetNillableClass sets the "class" field if the given value is not nil.
func (dzuo *DnsZoneUpdateOne) SetNillableClass(u *uint16) *DnsZoneUpdateOne {
	if u != nil {
		dzuo.SetClass(*u)
	}
	return dzuo
}

// AddClass adds u to the "class" field.
func (dzuo *DnsZoneUpdateOne) AddClass(u int16) *DnsZoneUpdateOne {
	dzuo.mutation.AddClass(u)
	return dzuo
}

// SetTTL sets the "ttl" field.
func (dzuo *DnsZoneUpdateOne) SetTTL(u uint32) *DnsZoneUpdateOne {
	dzuo.mutation.ResetTTL()
	dzuo.mutation.SetTTL(u)
	return dzuo
}

// SetNillableTTL sets the "ttl" field if the given value is not nil.
func (dzuo *DnsZoneUpdateOne) SetNillableTTL(u *uint32) *DnsZoneUpdateOne {
	if u != nil {
		dzuo.SetTTL(*u)
	}
	return dzuo
}

// AddTTL adds u to the "ttl" field.
func (dzuo *DnsZoneUpdateOne) AddTTL(u int32) *DnsZoneUpdateOne {
	dzuo.mutation.AddTTL(u)
	return dzuo
}

// SetNs sets the "ns" field.
func (dzuo *DnsZoneUpdateOne) SetNs(s string) *DnsZoneUpdateOne {
	dzuo.mutation.SetNs(s)
	return dzuo
}

// SetMbox sets the "mbox" field.
func (dzuo *DnsZoneUpdateOne) SetMbox(s string) *DnsZoneUpdateOne {
	dzuo.mutation.SetMbox(s)
	return dzuo
}

// SetSerial sets the "serial" field.
func (dzuo *DnsZoneUpdateOne) SetSerial(u uint32) *DnsZoneUpdateOne {
	dzuo.mutation.ResetSerial()
	dzuo.mutation.SetSerial(u)
	return dzuo
}

// AddSerial adds u to the "serial" field.
func (dzuo *DnsZoneUpdateOne) AddSerial(u int32) *DnsZoneUpdateOne {
	dzuo.mutation.AddSerial(u)
	return dzuo
}

// SetRefresh sets the "refresh" field.
func (dzuo *DnsZoneUpdateOne) SetRefresh(u uint32) *DnsZoneUpdateOne {
	dzuo.mutation.ResetRefresh()
	dzuo.mutation.SetRefresh(u)
	return dzuo
}

// SetNillableRefresh sets the "refresh" field if the given value is not nil.
func (dzuo *DnsZoneUpdateOne) SetNillableRefresh(u *uint32) *DnsZoneUpdateOne {
	if u != nil {
		dzuo.SetRefresh(*u)
	}
	return dzuo
}

// AddRefresh adds u to the "refresh" field.
func (dzuo *DnsZoneUpdateOne) AddRefresh(u int32) *DnsZoneUpdateOne {
	dzuo.mutation.AddRefresh(u)
	return dzuo
}

// SetRetry sets the "retry" field.
func (dzuo *DnsZoneUpdateOne) SetRetry(u uint32) *DnsZoneUpdateOne {
	dzuo.mutation.ResetRetry()
	dzuo.mutation.SetRetry(u)
	return dzuo
}

// SetNillableRetry sets the "retry" field if the given value is not nil.
func (dzuo *DnsZoneUpdateOne) SetNillableRetry(u *uint32) *DnsZoneUpdateOne {
	if u != nil {
		dzuo.SetRetry(*u)
	}
	return dzuo
}

// AddRetry adds u to the "retry" field.
func (dzuo *DnsZoneUpdateOne) AddRetry(u int32) *DnsZoneUpdateOne {
	dzuo.mutation.AddRetry(u)
	return dzuo
}

// SetExpire sets the "expire" field.
func (dzuo *DnsZoneUpdateOne) SetExpire(u uint32) *DnsZoneUpdateOne {
	dzuo.mutation.ResetExpire()
	dzuo.mutation.SetExpire(u)
	return dzuo
}

// SetNillableExpire sets the "expire" field if the given value is not nil.
func (dzuo *DnsZoneUpdateOne) SetNillableExpire(u *uint32) *DnsZoneUpdateOne {
	if u != nil {
		dzuo.SetExpire(*u)
	}
	return dzuo
}

// AddExpire adds u to the "expire" field.
func (dzuo *DnsZoneUpdateOne) AddExpire(u int32) *DnsZoneUpdateOne {
	dzuo.mutation.AddExpire(u)
	return dzuo
}

// SetMinttl sets the "minttl" field.
func (dzuo *DnsZoneUpdateOne) SetMinttl(u uint32) *DnsZoneUpdateOne {
	dzuo.mutation.ResetMinttl()
	dzuo.mutation.SetMinttl(u)
	return dzuo
}

// SetNillableMinttl sets the "minttl" field if the given value is not nil.
func (dzuo *DnsZoneUpdateOne) SetNillableMinttl(u *uint32) *DnsZoneUpdateOne {
	if u != nil {
		dzuo.SetMinttl(*u)
	}
	return dzuo
}

// AddMinttl adds u to the "minttl" field.
func (dzuo *DnsZoneUpdateOne) AddMinttl(u int32) *DnsZoneUpdateOne {
	dzuo.mutation.AddMinttl(u)
	return dzuo
}

// SetActivated sets the "activated" field.
func (dzuo *DnsZoneUpdateOne) SetActivated(b bool) *DnsZoneUpdateOne {
	dzuo.mutation.SetActivated(b)
	return dzuo
}

// SetNillableActivated sets the "activated" field if the given value is not nil.
func (dzuo *DnsZoneUpdateOne) SetNillableActivated(b *bool) *DnsZoneUpdateOne {
	if b != nil {
		dzuo.SetActivated(*b)
	}
	return dzuo
}

// AddRecordIDs adds the "records" edge to the DnsRR entity by IDs.
func (dzuo *DnsZoneUpdateOne) AddRecordIDs(ids ...xid.ID) *DnsZoneUpdateOne {
	dzuo.mutation.AddRecordIDs(ids...)
	return dzuo
}

// AddRecords adds the "records" edges to the DnsRR entity.
func (dzuo *DnsZoneUpdateOne) AddRecords(d ...*DnsRR) *DnsZoneUpdateOne {
	ids := make([]xid.ID, len(d))
	for i := range d {
		ids[i] = d[i].ID
	}
	return dzuo.AddRecordIDs(ids...)
}

// Mutation returns the DnsZoneMutation object of the builder.
func (dzuo *DnsZoneUpdateOne) Mutation() *DnsZoneMutation {
	return dzuo.mutation
}

// ClearRecords clears all "records" edges to the DnsRR entity.
func (dzuo *DnsZoneUpdateOne) ClearRecords() *DnsZoneUpdateOne {
	dzuo.mutation.ClearRecords()
	return dzuo
}

// RemoveRecordIDs removes the "records" edge to DnsRR entities by IDs.
func (dzuo *DnsZoneUpdateOne) RemoveRecordIDs(ids ...xid.ID) *DnsZoneUpdateOne {
	dzuo.mutation.RemoveRecordIDs(ids...)
	return dzuo
}

// RemoveRecords removes "records" edges to DnsRR entities.
func (dzuo *DnsZoneUpdateOne) RemoveRecords(d ...*DnsRR) *DnsZoneUpdateOne {
	ids := make([]xid.ID, len(d))
	for i := range d {
		ids[i] = d[i].ID
	}
	return dzuo.RemoveRecordIDs(ids...)
}

// Where appends a list predicates to the DnsZoneUpdate builder.
func (dzuo *DnsZoneUpdateOne) Where(ps ...predicate.DnsZone) *DnsZoneUpdateOne {
	dzuo.mutation.Where(ps...)
	return dzuo
}

// Select allows selecting one or more fields (columns) of the returned entity.
// The default is selecting all fields defined in the entity schema.
func (dzuo *DnsZoneUpdateOne) Select(field string, fields ...string) *DnsZoneUpdateOne {
	dzuo.fields = append([]string{field}, fields...)
	return dzuo
}

// Save executes the query and returns the updated DnsZone entity.
func (dzuo *DnsZoneUpdateOne) Save(ctx context.Context) (*DnsZone, error) {
	dzuo.defaults()
	return withHooks[*DnsZone, DnsZoneMutation](ctx, dzuo.sqlSave, dzuo.mutation, dzuo.hooks)
}

// SaveX is like Save, but panics if an error occurs.
func (dzuo *DnsZoneUpdateOne) SaveX(ctx context.Context) *DnsZone {
	node, err := dzuo.Save(ctx)
	if err != nil {
		panic(err)
	}
	return node
}

// Exec executes the query on the entity.
func (dzuo *DnsZoneUpdateOne) Exec(ctx context.Context) error {
	_, err := dzuo.Save(ctx)
	return err
}

// ExecX is like Exec, but panics if an error occurs.
func (dzuo *DnsZoneUpdateOne) ExecX(ctx context.Context) {
	if err := dzuo.Exec(ctx); err != nil {
		panic(err)
	}
}

// defaults sets the default values of the builder before save.
func (dzuo *DnsZoneUpdateOne) defaults() {
	if _, ok := dzuo.mutation.UpdatedAt(); !ok {
		v := dnszone.UpdateDefaultUpdatedAt()
		dzuo.mutation.SetUpdatedAt(v)
	}
}

// check runs all checks and user-defined validators on the builder.
func (dzuo *DnsZoneUpdateOne) check() error {
	if v, ok := dzuo.mutation.Rrtype(); ok {
		if err := dnszone.RrtypeValidator(v); err != nil {
			return &ValidationError{Name: "rrtype", err: fmt.Errorf(`ent: validator failed for field "DnsZone.rrtype": %w`, err)}
		}
	}
	if v, ok := dzuo.mutation.Class(); ok {
		if err := dnszone.ClassValidator(v); err != nil {
			return &ValidationError{Name: "class", err: fmt.Errorf(`ent: validator failed for field "DnsZone.class": %w`, err)}
		}
	}
	if v, ok := dzuo.mutation.TTL(); ok {
		if err := dnszone.TTLValidator(v); err != nil {
			return &ValidationError{Name: "ttl", err: fmt.Errorf(`ent: validator failed for field "DnsZone.ttl": %w`, err)}
		}
	}
	if v, ok := dzuo.mutation.Ns(); ok {
		if err := dnszone.NsValidator(v); err != nil {
			return &ValidationError{Name: "ns", err: fmt.Errorf(`ent: validator failed for field "DnsZone.ns": %w`, err)}
		}
	}
	if v, ok := dzuo.mutation.Mbox(); ok {
		if err := dnszone.MboxValidator(v); err != nil {
			return &ValidationError{Name: "mbox", err: fmt.Errorf(`ent: validator failed for field "DnsZone.mbox": %w`, err)}
		}
	}
	if v, ok := dzuo.mutation.Serial(); ok {
		if err := dnszone.SerialValidator(v); err != nil {
			return &ValidationError{Name: "serial", err: fmt.Errorf(`ent: validator failed for field "DnsZone.serial": %w`, err)}
		}
	}
	if v, ok := dzuo.mutation.Refresh(); ok {
		if err := dnszone.RefreshValidator(v); err != nil {
			return &ValidationError{Name: "refresh", err: fmt.Errorf(`ent: validator failed for field "DnsZone.refresh": %w`, err)}
		}
	}
	if v, ok := dzuo.mutation.Retry(); ok {
		if err := dnszone.RetryValidator(v); err != nil {
			return &ValidationError{Name: "retry", err: fmt.Errorf(`ent: validator failed for field "DnsZone.retry": %w`, err)}
		}
	}
	if v, ok := dzuo.mutation.Expire(); ok {
		if err := dnszone.ExpireValidator(v); err != nil {
			return &ValidationError{Name: "expire", err: fmt.Errorf(`ent: validator failed for field "DnsZone.expire": %w`, err)}
		}
	}
	if v, ok := dzuo.mutation.Minttl(); ok {
		if err := dnszone.MinttlValidator(v); err != nil {
			return &ValidationError{Name: "minttl", err: fmt.Errorf(`ent: validator failed for field "DnsZone.minttl": %w`, err)}
		}
	}
	return nil
}

func (dzuo *DnsZoneUpdateOne) sqlSave(ctx context.Context) (_node *DnsZone, err error) {
	if err := dzuo.check(); err != nil {
		return _node, err
	}
	_spec := sqlgraph.NewUpdateSpec(dnszone.Table, dnszone.Columns, sqlgraph.NewFieldSpec(dnszone.FieldID, field.TypeString))
	id, ok := dzuo.mutation.ID()
	if !ok {
		return nil, &ValidationError{Name: "id", err: errors.New(`ent: missing "DnsZone.id" for update`)}
	}
	_spec.Node.ID.Value = id
	if fields := dzuo.fields; len(fields) > 0 {
		_spec.Node.Columns = make([]string, 0, len(fields))
		_spec.Node.Columns = append(_spec.Node.Columns, dnszone.FieldID)
		for _, f := range fields {
			if !dnszone.ValidColumn(f) {
				return nil, &ValidationError{Name: f, err: fmt.Errorf("ent: invalid field %q for query", f)}
			}
			if f != dnszone.FieldID {
				_spec.Node.Columns = append(_spec.Node.Columns, f)
			}
		}
	}
	if ps := dzuo.mutation.predicates; len(ps) > 0 {
		_spec.Predicate = func(selector *sql.Selector) {
			for i := range ps {
				ps[i](selector)
			}
		}
	}
	if value, ok := dzuo.mutation.UpdatedAt(); ok {
		_spec.SetField(dnszone.FieldUpdatedAt, field.TypeTime, value)
	}
	if value, ok := dzuo.mutation.Rrtype(); ok {
		_spec.SetField(dnszone.FieldRrtype, field.TypeUint16, value)
	}
	if value, ok := dzuo.mutation.AddedRrtype(); ok {
		_spec.AddField(dnszone.FieldRrtype, field.TypeUint16, value)
	}
	if value, ok := dzuo.mutation.Class(); ok {
		_spec.SetField(dnszone.FieldClass, field.TypeUint16, value)
	}
	if value, ok := dzuo.mutation.AddedClass(); ok {
		_spec.AddField(dnszone.FieldClass, field.TypeUint16, value)
	}
	if value, ok := dzuo.mutation.TTL(); ok {
		_spec.SetField(dnszone.FieldTTL, field.TypeUint32, value)
	}
	if value, ok := dzuo.mutation.AddedTTL(); ok {
		_spec.AddField(dnszone.FieldTTL, field.TypeUint32, value)
	}
	if value, ok := dzuo.mutation.Ns(); ok {
		_spec.SetField(dnszone.FieldNs, field.TypeString, value)
	}
	if value, ok := dzuo.mutation.Mbox(); ok {
		_spec.SetField(dnszone.FieldMbox, field.TypeString, value)
	}
	if value, ok := dzuo.mutation.Serial(); ok {
		_spec.SetField(dnszone.FieldSerial, field.TypeUint32, value)
	}
	if value, ok := dzuo.mutation.AddedSerial(); ok {
		_spec.AddField(dnszone.FieldSerial, field.TypeUint32, value)
	}
	if value, ok := dzuo.mutation.Refresh(); ok {
		_spec.SetField(dnszone.FieldRefresh, field.TypeUint32, value)
	}
	if value, ok := dzuo.mutation.AddedRefresh(); ok {
		_spec.AddField(dnszone.FieldRefresh, field.TypeUint32, value)
	}
	if value, ok := dzuo.mutation.Retry(); ok {
		_spec.SetField(dnszone.FieldRetry, field.TypeUint32, value)
	}
	if value, ok := dzuo.mutation.AddedRetry(); ok {
		_spec.AddField(dnszone.FieldRetry, field.TypeUint32, value)
	}
	if value, ok := dzuo.mutation.Expire(); ok {
		_spec.SetField(dnszone.FieldExpire, field.TypeUint32, value)
	}
	if value, ok := dzuo.mutation.AddedExpire(); ok {
		_spec.AddField(dnszone.FieldExpire, field.TypeUint32, value)
	}
	if value, ok := dzuo.mutation.Minttl(); ok {
		_spec.SetField(dnszone.FieldMinttl, field.TypeUint32, value)
	}
	if value, ok := dzuo.mutation.AddedMinttl(); ok {
		_spec.AddField(dnszone.FieldMinttl, field.TypeUint32, value)
	}
	if value, ok := dzuo.mutation.Activated(); ok {
		_spec.SetField(dnszone.FieldActivated, field.TypeBool, value)
	}
	if dzuo.mutation.RecordsCleared() {
		edge := &sqlgraph.EdgeSpec{
			Rel:     sqlgraph.O2M,
			Inverse: false,
			Table:   dnszone.RecordsTable,
			Columns: []string{dnszone.RecordsColumn},
			Bidi:    false,
			Target: &sqlgraph.EdgeTarget{
				IDSpec: sqlgraph.NewFieldSpec(dnsrr.FieldID, field.TypeString),
			},
		}
		_spec.Edges.Clear = append(_spec.Edges.Clear, edge)
	}
	if nodes := dzuo.mutation.RemovedRecordsIDs(); len(nodes) > 0 && !dzuo.mutation.RecordsCleared() {
		edge := &sqlgraph.EdgeSpec{
			Rel:     sqlgraph.O2M,
			Inverse: false,
			Table:   dnszone.RecordsTable,
			Columns: []string{dnszone.RecordsColumn},
			Bidi:    false,
			Target: &sqlgraph.EdgeTarget{
				IDSpec: sqlgraph.NewFieldSpec(dnsrr.FieldID, field.TypeString),
			},
		}
		for _, k := range nodes {
			edge.Target.Nodes = append(edge.Target.Nodes, k)
		}
		_spec.Edges.Clear = append(_spec.Edges.Clear, edge)
	}
	if nodes := dzuo.mutation.RecordsIDs(); len(nodes) > 0 {
		edge := &sqlgraph.EdgeSpec{
			Rel:     sqlgraph.O2M,
			Inverse: false,
			Table:   dnszone.RecordsTable,
			Columns: []string{dnszone.RecordsColumn},
			Bidi:    false,
			Target: &sqlgraph.EdgeTarget{
				IDSpec: sqlgraph.NewFieldSpec(dnsrr.FieldID, field.TypeString),
			},
		}
		for _, k := range nodes {
			edge.Target.Nodes = append(edge.Target.Nodes, k)
		}
		_spec.Edges.Add = append(_spec.Edges.Add, edge)
	}
	_node = &DnsZone{config: dzuo.config}
	_spec.Assign = _node.assignValues
	_spec.ScanValues = _node.scanValues
	if err = sqlgraph.UpdateNode(ctx, dzuo.driver, _spec); err != nil {
		if _, ok := err.(*sqlgraph.NotFoundError); ok {
			err = &NotFoundError{dnszone.Label}
		} else if sqlgraph.IsConstraintError(err) {
			err = &ConstraintError{msg: err.Error(), wrap: err}
		}
		return nil, err
	}
	dzuo.mutation.done = true
	return _node, nil
}
