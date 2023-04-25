// Code generated by ent, DO NOT EDIT.

package ent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/schema/field"
	"github.com/coredns/coredns/plugin/atlas/ent/dnsrr"
	"github.com/coredns/coredns/plugin/atlas/ent/dnszone"
	"github.com/rs/xid"
)

// DnsRRCreate is the builder for creating a DnsRR entity.
type DnsRRCreate struct {
	config
	mutation *DnsRRMutation
	hooks    []Hook
	conflict []sql.ConflictOption
}

// SetCreatedAt sets the "created_at" field.
func (drc *DnsRRCreate) SetCreatedAt(t time.Time) *DnsRRCreate {
	drc.mutation.SetCreatedAt(t)
	return drc
}

// SetNillableCreatedAt sets the "created_at" field if the given value is not nil.
func (drc *DnsRRCreate) SetNillableCreatedAt(t *time.Time) *DnsRRCreate {
	if t != nil {
		drc.SetCreatedAt(*t)
	}
	return drc
}

// SetUpdatedAt sets the "updated_at" field.
func (drc *DnsRRCreate) SetUpdatedAt(t time.Time) *DnsRRCreate {
	drc.mutation.SetUpdatedAt(t)
	return drc
}

// SetNillableUpdatedAt sets the "updated_at" field if the given value is not nil.
func (drc *DnsRRCreate) SetNillableUpdatedAt(t *time.Time) *DnsRRCreate {
	if t != nil {
		drc.SetUpdatedAt(*t)
	}
	return drc
}

// SetName sets the "name" field.
func (drc *DnsRRCreate) SetName(s string) *DnsRRCreate {
	drc.mutation.SetName(s)
	return drc
}

// SetRrtype sets the "rrtype" field.
func (drc *DnsRRCreate) SetRrtype(u uint16) *DnsRRCreate {
	drc.mutation.SetRrtype(u)
	return drc
}

// SetRrdata sets the "rrdata" field.
func (drc *DnsRRCreate) SetRrdata(s string) *DnsRRCreate {
	drc.mutation.SetRrdata(s)
	return drc
}

// SetClass sets the "class" field.
func (drc *DnsRRCreate) SetClass(u uint16) *DnsRRCreate {
	drc.mutation.SetClass(u)
	return drc
}

// SetNillableClass sets the "class" field if the given value is not nil.
func (drc *DnsRRCreate) SetNillableClass(u *uint16) *DnsRRCreate {
	if u != nil {
		drc.SetClass(*u)
	}
	return drc
}

// SetTTL sets the "ttl" field.
func (drc *DnsRRCreate) SetTTL(u uint32) *DnsRRCreate {
	drc.mutation.SetTTL(u)
	return drc
}

// SetNillableTTL sets the "ttl" field if the given value is not nil.
func (drc *DnsRRCreate) SetNillableTTL(u *uint32) *DnsRRCreate {
	if u != nil {
		drc.SetTTL(*u)
	}
	return drc
}

// SetActivated sets the "activated" field.
func (drc *DnsRRCreate) SetActivated(b bool) *DnsRRCreate {
	drc.mutation.SetActivated(b)
	return drc
}

// SetNillableActivated sets the "activated" field if the given value is not nil.
func (drc *DnsRRCreate) SetNillableActivated(b *bool) *DnsRRCreate {
	if b != nil {
		drc.SetActivated(*b)
	}
	return drc
}

// SetID sets the "id" field.
func (drc *DnsRRCreate) SetID(x xid.ID) *DnsRRCreate {
	drc.mutation.SetID(x)
	return drc
}

// SetNillableID sets the "id" field if the given value is not nil.
func (drc *DnsRRCreate) SetNillableID(x *xid.ID) *DnsRRCreate {
	if x != nil {
		drc.SetID(*x)
	}
	return drc
}

// SetZoneID sets the "zone" edge to the DnsZone entity by ID.
func (drc *DnsRRCreate) SetZoneID(id xid.ID) *DnsRRCreate {
	drc.mutation.SetZoneID(id)
	return drc
}

// SetZone sets the "zone" edge to the DnsZone entity.
func (drc *DnsRRCreate) SetZone(d *DnsZone) *DnsRRCreate {
	return drc.SetZoneID(d.ID)
}

// Mutation returns the DnsRRMutation object of the builder.
func (drc *DnsRRCreate) Mutation() *DnsRRMutation {
	return drc.mutation
}

// Save creates the DnsRR in the database.
func (drc *DnsRRCreate) Save(ctx context.Context) (*DnsRR, error) {
	drc.defaults()
	return withHooks[*DnsRR, DnsRRMutation](ctx, drc.sqlSave, drc.mutation, drc.hooks)
}

// SaveX calls Save and panics if Save returns an error.
func (drc *DnsRRCreate) SaveX(ctx context.Context) *DnsRR {
	v, err := drc.Save(ctx)
	if err != nil {
		panic(err)
	}
	return v
}

// Exec executes the query.
func (drc *DnsRRCreate) Exec(ctx context.Context) error {
	_, err := drc.Save(ctx)
	return err
}

// ExecX is like Exec, but panics if an error occurs.
func (drc *DnsRRCreate) ExecX(ctx context.Context) {
	if err := drc.Exec(ctx); err != nil {
		panic(err)
	}
}

// defaults sets the default values of the builder before save.
func (drc *DnsRRCreate) defaults() {
	if _, ok := drc.mutation.CreatedAt(); !ok {
		v := dnsrr.DefaultCreatedAt()
		drc.mutation.SetCreatedAt(v)
	}
	if _, ok := drc.mutation.UpdatedAt(); !ok {
		v := dnsrr.DefaultUpdatedAt()
		drc.mutation.SetUpdatedAt(v)
	}
	if _, ok := drc.mutation.Class(); !ok {
		v := dnsrr.DefaultClass
		drc.mutation.SetClass(v)
	}
	if _, ok := drc.mutation.TTL(); !ok {
		v := dnsrr.DefaultTTL
		drc.mutation.SetTTL(v)
	}
	if _, ok := drc.mutation.Activated(); !ok {
		v := dnsrr.DefaultActivated
		drc.mutation.SetActivated(v)
	}
	if _, ok := drc.mutation.ID(); !ok {
		v := dnsrr.DefaultID()
		drc.mutation.SetID(v)
	}
}

// check runs all checks and user-defined validators on the builder.
func (drc *DnsRRCreate) check() error {
	if _, ok := drc.mutation.CreatedAt(); !ok {
		return &ValidationError{Name: "created_at", err: errors.New(`ent: missing required field "DnsRR.created_at"`)}
	}
	if _, ok := drc.mutation.UpdatedAt(); !ok {
		return &ValidationError{Name: "updated_at", err: errors.New(`ent: missing required field "DnsRR.updated_at"`)}
	}
	if _, ok := drc.mutation.Name(); !ok {
		return &ValidationError{Name: "name", err: errors.New(`ent: missing required field "DnsRR.name"`)}
	}
	if v, ok := drc.mutation.Name(); ok {
		if err := dnsrr.NameValidator(v); err != nil {
			return &ValidationError{Name: "name", err: fmt.Errorf(`ent: validator failed for field "DnsRR.name": %w`, err)}
		}
	}
	if _, ok := drc.mutation.Rrtype(); !ok {
		return &ValidationError{Name: "rrtype", err: errors.New(`ent: missing required field "DnsRR.rrtype"`)}
	}
	if v, ok := drc.mutation.Rrtype(); ok {
		if err := dnsrr.RrtypeValidator(v); err != nil {
			return &ValidationError{Name: "rrtype", err: fmt.Errorf(`ent: validator failed for field "DnsRR.rrtype": %w`, err)}
		}
	}
	if _, ok := drc.mutation.Rrdata(); !ok {
		return &ValidationError{Name: "rrdata", err: errors.New(`ent: missing required field "DnsRR.rrdata"`)}
	}
	if _, ok := drc.mutation.Class(); !ok {
		return &ValidationError{Name: "class", err: errors.New(`ent: missing required field "DnsRR.class"`)}
	}
	if v, ok := drc.mutation.Class(); ok {
		if err := dnsrr.ClassValidator(v); err != nil {
			return &ValidationError{Name: "class", err: fmt.Errorf(`ent: validator failed for field "DnsRR.class": %w`, err)}
		}
	}
	if _, ok := drc.mutation.TTL(); !ok {
		return &ValidationError{Name: "ttl", err: errors.New(`ent: missing required field "DnsRR.ttl"`)}
	}
	if v, ok := drc.mutation.TTL(); ok {
		if err := dnsrr.TTLValidator(v); err != nil {
			return &ValidationError{Name: "ttl", err: fmt.Errorf(`ent: validator failed for field "DnsRR.ttl": %w`, err)}
		}
	}
	if _, ok := drc.mutation.Activated(); !ok {
		return &ValidationError{Name: "activated", err: errors.New(`ent: missing required field "DnsRR.activated"`)}
	}
	if _, ok := drc.mutation.ZoneID(); !ok {
		return &ValidationError{Name: "zone", err: errors.New(`ent: missing required edge "DnsRR.zone"`)}
	}
	return nil
}

func (drc *DnsRRCreate) sqlSave(ctx context.Context) (*DnsRR, error) {
	if err := drc.check(); err != nil {
		return nil, err
	}
	_node, _spec := drc.createSpec()
	if err := sqlgraph.CreateNode(ctx, drc.driver, _spec); err != nil {
		if sqlgraph.IsConstraintError(err) {
			err = &ConstraintError{msg: err.Error(), wrap: err}
		}
		return nil, err
	}
	if _spec.ID.Value != nil {
		if id, ok := _spec.ID.Value.(*xid.ID); ok {
			_node.ID = *id
		} else if err := _node.ID.Scan(_spec.ID.Value); err != nil {
			return nil, err
		}
	}
	drc.mutation.id = &_node.ID
	drc.mutation.done = true
	return _node, nil
}

func (drc *DnsRRCreate) createSpec() (*DnsRR, *sqlgraph.CreateSpec) {
	var (
		_node = &DnsRR{config: drc.config}
		_spec = sqlgraph.NewCreateSpec(dnsrr.Table, sqlgraph.NewFieldSpec(dnsrr.FieldID, field.TypeString))
	)
	_spec.OnConflict = drc.conflict
	if id, ok := drc.mutation.ID(); ok {
		_node.ID = id
		_spec.ID.Value = &id
	}
	if value, ok := drc.mutation.CreatedAt(); ok {
		_spec.SetField(dnsrr.FieldCreatedAt, field.TypeTime, value)
		_node.CreatedAt = &value
	}
	if value, ok := drc.mutation.UpdatedAt(); ok {
		_spec.SetField(dnsrr.FieldUpdatedAt, field.TypeTime, value)
		_node.UpdatedAt = &value
	}
	if value, ok := drc.mutation.Name(); ok {
		_spec.SetField(dnsrr.FieldName, field.TypeString, value)
		_node.Name = value
	}
	if value, ok := drc.mutation.Rrtype(); ok {
		_spec.SetField(dnsrr.FieldRrtype, field.TypeUint16, value)
		_node.Rrtype = value
	}
	if value, ok := drc.mutation.Rrdata(); ok {
		_spec.SetField(dnsrr.FieldRrdata, field.TypeString, value)
		_node.Rrdata = value
	}
	if value, ok := drc.mutation.Class(); ok {
		_spec.SetField(dnsrr.FieldClass, field.TypeUint16, value)
		_node.Class = value
	}
	if value, ok := drc.mutation.TTL(); ok {
		_spec.SetField(dnsrr.FieldTTL, field.TypeUint32, value)
		_node.TTL = value
	}
	if value, ok := drc.mutation.Activated(); ok {
		_spec.SetField(dnsrr.FieldActivated, field.TypeBool, value)
		_node.Activated = &value
	}
	if nodes := drc.mutation.ZoneIDs(); len(nodes) > 0 {
		edge := &sqlgraph.EdgeSpec{
			Rel:     sqlgraph.M2O,
			Inverse: true,
			Table:   dnsrr.ZoneTable,
			Columns: []string{dnsrr.ZoneColumn},
			Bidi:    false,
			Target: &sqlgraph.EdgeTarget{
				IDSpec: sqlgraph.NewFieldSpec(dnszone.FieldID, field.TypeString),
			},
		}
		for _, k := range nodes {
			edge.Target.Nodes = append(edge.Target.Nodes, k)
		}
		_node.dns_zone_records = &nodes[0]
		_spec.Edges = append(_spec.Edges, edge)
	}
	return _node, _spec
}

// OnConflict allows configuring the `ON CONFLICT` / `ON DUPLICATE KEY` clause
// of the `INSERT` statement. For example:
//
//	client.DnsRR.Create().
//		SetCreatedAt(v).
//		OnConflict(
//			// Update the row with the new values
//			// the was proposed for insertion.
//			sql.ResolveWithNewValues(),
//		).
//		// Override some of the fields with custom
//		// update values.
//		Update(func(u *ent.DnsRRUpsert) {
//			SetCreatedAt(v+v).
//		}).
//		Exec(ctx)
func (drc *DnsRRCreate) OnConflict(opts ...sql.ConflictOption) *DnsRRUpsertOne {
	drc.conflict = opts
	return &DnsRRUpsertOne{
		create: drc,
	}
}

// OnConflictColumns calls `OnConflict` and configures the columns
// as conflict target. Using this option is equivalent to using:
//
//	client.DnsRR.Create().
//		OnConflict(sql.ConflictColumns(columns...)).
//		Exec(ctx)
func (drc *DnsRRCreate) OnConflictColumns(columns ...string) *DnsRRUpsertOne {
	drc.conflict = append(drc.conflict, sql.ConflictColumns(columns...))
	return &DnsRRUpsertOne{
		create: drc,
	}
}

type (
	// DnsRRUpsertOne is the builder for "upsert"-ing
	//  one DnsRR node.
	DnsRRUpsertOne struct {
		create *DnsRRCreate
	}

	// DnsRRUpsert is the "OnConflict" setter.
	DnsRRUpsert struct {
		*sql.UpdateSet
	}
)

// SetUpdatedAt sets the "updated_at" field.
func (u *DnsRRUpsert) SetUpdatedAt(v time.Time) *DnsRRUpsert {
	u.Set(dnsrr.FieldUpdatedAt, v)
	return u
}

// UpdateUpdatedAt sets the "updated_at" field to the value that was provided on create.
func (u *DnsRRUpsert) UpdateUpdatedAt() *DnsRRUpsert {
	u.SetExcluded(dnsrr.FieldUpdatedAt)
	return u
}

// SetRrtype sets the "rrtype" field.
func (u *DnsRRUpsert) SetRrtype(v uint16) *DnsRRUpsert {
	u.Set(dnsrr.FieldRrtype, v)
	return u
}

// UpdateRrtype sets the "rrtype" field to the value that was provided on create.
func (u *DnsRRUpsert) UpdateRrtype() *DnsRRUpsert {
	u.SetExcluded(dnsrr.FieldRrtype)
	return u
}

// AddRrtype adds v to the "rrtype" field.
func (u *DnsRRUpsert) AddRrtype(v uint16) *DnsRRUpsert {
	u.Add(dnsrr.FieldRrtype, v)
	return u
}

// SetRrdata sets the "rrdata" field.
func (u *DnsRRUpsert) SetRrdata(v string) *DnsRRUpsert {
	u.Set(dnsrr.FieldRrdata, v)
	return u
}

// UpdateRrdata sets the "rrdata" field to the value that was provided on create.
func (u *DnsRRUpsert) UpdateRrdata() *DnsRRUpsert {
	u.SetExcluded(dnsrr.FieldRrdata)
	return u
}

// SetClass sets the "class" field.
func (u *DnsRRUpsert) SetClass(v uint16) *DnsRRUpsert {
	u.Set(dnsrr.FieldClass, v)
	return u
}

// UpdateClass sets the "class" field to the value that was provided on create.
func (u *DnsRRUpsert) UpdateClass() *DnsRRUpsert {
	u.SetExcluded(dnsrr.FieldClass)
	return u
}

// AddClass adds v to the "class" field.
func (u *DnsRRUpsert) AddClass(v uint16) *DnsRRUpsert {
	u.Add(dnsrr.FieldClass, v)
	return u
}

// SetTTL sets the "ttl" field.
func (u *DnsRRUpsert) SetTTL(v uint32) *DnsRRUpsert {
	u.Set(dnsrr.FieldTTL, v)
	return u
}

// UpdateTTL sets the "ttl" field to the value that was provided on create.
func (u *DnsRRUpsert) UpdateTTL() *DnsRRUpsert {
	u.SetExcluded(dnsrr.FieldTTL)
	return u
}

// AddTTL adds v to the "ttl" field.
func (u *DnsRRUpsert) AddTTL(v uint32) *DnsRRUpsert {
	u.Add(dnsrr.FieldTTL, v)
	return u
}

// SetActivated sets the "activated" field.
func (u *DnsRRUpsert) SetActivated(v bool) *DnsRRUpsert {
	u.Set(dnsrr.FieldActivated, v)
	return u
}

// UpdateActivated sets the "activated" field to the value that was provided on create.
func (u *DnsRRUpsert) UpdateActivated() *DnsRRUpsert {
	u.SetExcluded(dnsrr.FieldActivated)
	return u
}

// UpdateNewValues updates the mutable fields using the new values that were set on create except the ID field.
// Using this option is equivalent to using:
//
//	client.DnsRR.Create().
//		OnConflict(
//			sql.ResolveWithNewValues(),
//			sql.ResolveWith(func(u *sql.UpdateSet) {
//				u.SetIgnore(dnsrr.FieldID)
//			}),
//		).
//		Exec(ctx)
func (u *DnsRRUpsertOne) UpdateNewValues() *DnsRRUpsertOne {
	u.create.conflict = append(u.create.conflict, sql.ResolveWithNewValues())
	u.create.conflict = append(u.create.conflict, sql.ResolveWith(func(s *sql.UpdateSet) {
		if _, exists := u.create.mutation.ID(); exists {
			s.SetIgnore(dnsrr.FieldID)
		}
		if _, exists := u.create.mutation.CreatedAt(); exists {
			s.SetIgnore(dnsrr.FieldCreatedAt)
		}
		if _, exists := u.create.mutation.Name(); exists {
			s.SetIgnore(dnsrr.FieldName)
		}
	}))
	return u
}

// Ignore sets each column to itself in case of conflict.
// Using this option is equivalent to using:
//
//	client.DnsRR.Create().
//	    OnConflict(sql.ResolveWithIgnore()).
//	    Exec(ctx)
func (u *DnsRRUpsertOne) Ignore() *DnsRRUpsertOne {
	u.create.conflict = append(u.create.conflict, sql.ResolveWithIgnore())
	return u
}

// DoNothing configures the conflict_action to `DO NOTHING`.
// Supported only by SQLite and PostgreSQL.
func (u *DnsRRUpsertOne) DoNothing() *DnsRRUpsertOne {
	u.create.conflict = append(u.create.conflict, sql.DoNothing())
	return u
}

// Update allows overriding fields `UPDATE` values. See the DnsRRCreate.OnConflict
// documentation for more info.
func (u *DnsRRUpsertOne) Update(set func(*DnsRRUpsert)) *DnsRRUpsertOne {
	u.create.conflict = append(u.create.conflict, sql.ResolveWith(func(update *sql.UpdateSet) {
		set(&DnsRRUpsert{UpdateSet: update})
	}))
	return u
}

// SetUpdatedAt sets the "updated_at" field.
func (u *DnsRRUpsertOne) SetUpdatedAt(v time.Time) *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.SetUpdatedAt(v)
	})
}

// UpdateUpdatedAt sets the "updated_at" field to the value that was provided on create.
func (u *DnsRRUpsertOne) UpdateUpdatedAt() *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.UpdateUpdatedAt()
	})
}

// SetRrtype sets the "rrtype" field.
func (u *DnsRRUpsertOne) SetRrtype(v uint16) *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.SetRrtype(v)
	})
}

// AddRrtype adds v to the "rrtype" field.
func (u *DnsRRUpsertOne) AddRrtype(v uint16) *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.AddRrtype(v)
	})
}

// UpdateRrtype sets the "rrtype" field to the value that was provided on create.
func (u *DnsRRUpsertOne) UpdateRrtype() *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.UpdateRrtype()
	})
}

// SetRrdata sets the "rrdata" field.
func (u *DnsRRUpsertOne) SetRrdata(v string) *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.SetRrdata(v)
	})
}

// UpdateRrdata sets the "rrdata" field to the value that was provided on create.
func (u *DnsRRUpsertOne) UpdateRrdata() *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.UpdateRrdata()
	})
}

// SetClass sets the "class" field.
func (u *DnsRRUpsertOne) SetClass(v uint16) *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.SetClass(v)
	})
}

// AddClass adds v to the "class" field.
func (u *DnsRRUpsertOne) AddClass(v uint16) *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.AddClass(v)
	})
}

// UpdateClass sets the "class" field to the value that was provided on create.
func (u *DnsRRUpsertOne) UpdateClass() *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.UpdateClass()
	})
}

// SetTTL sets the "ttl" field.
func (u *DnsRRUpsertOne) SetTTL(v uint32) *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.SetTTL(v)
	})
}

// AddTTL adds v to the "ttl" field.
func (u *DnsRRUpsertOne) AddTTL(v uint32) *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.AddTTL(v)
	})
}

// UpdateTTL sets the "ttl" field to the value that was provided on create.
func (u *DnsRRUpsertOne) UpdateTTL() *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.UpdateTTL()
	})
}

// SetActivated sets the "activated" field.
func (u *DnsRRUpsertOne) SetActivated(v bool) *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.SetActivated(v)
	})
}

// UpdateActivated sets the "activated" field to the value that was provided on create.
func (u *DnsRRUpsertOne) UpdateActivated() *DnsRRUpsertOne {
	return u.Update(func(s *DnsRRUpsert) {
		s.UpdateActivated()
	})
}

// Exec executes the query.
func (u *DnsRRUpsertOne) Exec(ctx context.Context) error {
	if len(u.create.conflict) == 0 {
		return errors.New("ent: missing options for DnsRRCreate.OnConflict")
	}
	return u.create.Exec(ctx)
}

// ExecX is like Exec, but panics if an error occurs.
func (u *DnsRRUpsertOne) ExecX(ctx context.Context) {
	if err := u.create.Exec(ctx); err != nil {
		panic(err)
	}
}

// Exec executes the UPSERT query and returns the inserted/updated ID.
func (u *DnsRRUpsertOne) ID(ctx context.Context) (id xid.ID, err error) {
	if u.create.driver.Dialect() == dialect.MySQL {
		// In case of "ON CONFLICT", there is no way to get back non-numeric ID
		// fields from the database since MySQL does not support the RETURNING clause.
		return id, errors.New("ent: DnsRRUpsertOne.ID is not supported by MySQL driver. Use DnsRRUpsertOne.Exec instead")
	}
	node, err := u.create.Save(ctx)
	if err != nil {
		return id, err
	}
	return node.ID, nil
}

// IDX is like ID, but panics if an error occurs.
func (u *DnsRRUpsertOne) IDX(ctx context.Context) xid.ID {
	id, err := u.ID(ctx)
	if err != nil {
		panic(err)
	}
	return id
}

// DnsRRCreateBulk is the builder for creating many DnsRR entities in bulk.
type DnsRRCreateBulk struct {
	config
	builders []*DnsRRCreate
	conflict []sql.ConflictOption
}

// Save creates the DnsRR entities in the database.
func (drcb *DnsRRCreateBulk) Save(ctx context.Context) ([]*DnsRR, error) {
	specs := make([]*sqlgraph.CreateSpec, len(drcb.builders))
	nodes := make([]*DnsRR, len(drcb.builders))
	mutators := make([]Mutator, len(drcb.builders))
	for i := range drcb.builders {
		func(i int, root context.Context) {
			builder := drcb.builders[i]
			builder.defaults()
			var mut Mutator = MutateFunc(func(ctx context.Context, m Mutation) (Value, error) {
				mutation, ok := m.(*DnsRRMutation)
				if !ok {
					return nil, fmt.Errorf("unexpected mutation type %T", m)
				}
				if err := builder.check(); err != nil {
					return nil, err
				}
				builder.mutation = mutation
				nodes[i], specs[i] = builder.createSpec()
				var err error
				if i < len(mutators)-1 {
					_, err = mutators[i+1].Mutate(root, drcb.builders[i+1].mutation)
				} else {
					spec := &sqlgraph.BatchCreateSpec{Nodes: specs}
					spec.OnConflict = drcb.conflict
					// Invoke the actual operation on the latest mutation in the chain.
					if err = sqlgraph.BatchCreate(ctx, drcb.driver, spec); err != nil {
						if sqlgraph.IsConstraintError(err) {
							err = &ConstraintError{msg: err.Error(), wrap: err}
						}
					}
				}
				if err != nil {
					return nil, err
				}
				mutation.id = &nodes[i].ID
				mutation.done = true
				return nodes[i], nil
			})
			for i := len(builder.hooks) - 1; i >= 0; i-- {
				mut = builder.hooks[i](mut)
			}
			mutators[i] = mut
		}(i, ctx)
	}
	if len(mutators) > 0 {
		if _, err := mutators[0].Mutate(ctx, drcb.builders[0].mutation); err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

// SaveX is like Save, but panics if an error occurs.
func (drcb *DnsRRCreateBulk) SaveX(ctx context.Context) []*DnsRR {
	v, err := drcb.Save(ctx)
	if err != nil {
		panic(err)
	}
	return v
}

// Exec executes the query.
func (drcb *DnsRRCreateBulk) Exec(ctx context.Context) error {
	_, err := drcb.Save(ctx)
	return err
}

// ExecX is like Exec, but panics if an error occurs.
func (drcb *DnsRRCreateBulk) ExecX(ctx context.Context) {
	if err := drcb.Exec(ctx); err != nil {
		panic(err)
	}
}

// OnConflict allows configuring the `ON CONFLICT` / `ON DUPLICATE KEY` clause
// of the `INSERT` statement. For example:
//
//	client.DnsRR.CreateBulk(builders...).
//		OnConflict(
//			// Update the row with the new values
//			// the was proposed for insertion.
//			sql.ResolveWithNewValues(),
//		).
//		// Override some of the fields with custom
//		// update values.
//		Update(func(u *ent.DnsRRUpsert) {
//			SetCreatedAt(v+v).
//		}).
//		Exec(ctx)
func (drcb *DnsRRCreateBulk) OnConflict(opts ...sql.ConflictOption) *DnsRRUpsertBulk {
	drcb.conflict = opts
	return &DnsRRUpsertBulk{
		create: drcb,
	}
}

// OnConflictColumns calls `OnConflict` and configures the columns
// as conflict target. Using this option is equivalent to using:
//
//	client.DnsRR.Create().
//		OnConflict(sql.ConflictColumns(columns...)).
//		Exec(ctx)
func (drcb *DnsRRCreateBulk) OnConflictColumns(columns ...string) *DnsRRUpsertBulk {
	drcb.conflict = append(drcb.conflict, sql.ConflictColumns(columns...))
	return &DnsRRUpsertBulk{
		create: drcb,
	}
}

// DnsRRUpsertBulk is the builder for "upsert"-ing
// a bulk of DnsRR nodes.
type DnsRRUpsertBulk struct {
	create *DnsRRCreateBulk
}

// UpdateNewValues updates the mutable fields using the new values that
// were set on create. Using this option is equivalent to using:
//
//	client.DnsRR.Create().
//		OnConflict(
//			sql.ResolveWithNewValues(),
//			sql.ResolveWith(func(u *sql.UpdateSet) {
//				u.SetIgnore(dnsrr.FieldID)
//			}),
//		).
//		Exec(ctx)
func (u *DnsRRUpsertBulk) UpdateNewValues() *DnsRRUpsertBulk {
	u.create.conflict = append(u.create.conflict, sql.ResolveWithNewValues())
	u.create.conflict = append(u.create.conflict, sql.ResolveWith(func(s *sql.UpdateSet) {
		for _, b := range u.create.builders {
			if _, exists := b.mutation.ID(); exists {
				s.SetIgnore(dnsrr.FieldID)
			}
			if _, exists := b.mutation.CreatedAt(); exists {
				s.SetIgnore(dnsrr.FieldCreatedAt)
			}
			if _, exists := b.mutation.Name(); exists {
				s.SetIgnore(dnsrr.FieldName)
			}
		}
	}))
	return u
}

// Ignore sets each column to itself in case of conflict.
// Using this option is equivalent to using:
//
//	client.DnsRR.Create().
//		OnConflict(sql.ResolveWithIgnore()).
//		Exec(ctx)
func (u *DnsRRUpsertBulk) Ignore() *DnsRRUpsertBulk {
	u.create.conflict = append(u.create.conflict, sql.ResolveWithIgnore())
	return u
}

// DoNothing configures the conflict_action to `DO NOTHING`.
// Supported only by SQLite and PostgreSQL.
func (u *DnsRRUpsertBulk) DoNothing() *DnsRRUpsertBulk {
	u.create.conflict = append(u.create.conflict, sql.DoNothing())
	return u
}

// Update allows overriding fields `UPDATE` values. See the DnsRRCreateBulk.OnConflict
// documentation for more info.
func (u *DnsRRUpsertBulk) Update(set func(*DnsRRUpsert)) *DnsRRUpsertBulk {
	u.create.conflict = append(u.create.conflict, sql.ResolveWith(func(update *sql.UpdateSet) {
		set(&DnsRRUpsert{UpdateSet: update})
	}))
	return u
}

// SetUpdatedAt sets the "updated_at" field.
func (u *DnsRRUpsertBulk) SetUpdatedAt(v time.Time) *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.SetUpdatedAt(v)
	})
}

// UpdateUpdatedAt sets the "updated_at" field to the value that was provided on create.
func (u *DnsRRUpsertBulk) UpdateUpdatedAt() *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.UpdateUpdatedAt()
	})
}

// SetRrtype sets the "rrtype" field.
func (u *DnsRRUpsertBulk) SetRrtype(v uint16) *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.SetRrtype(v)
	})
}

// AddRrtype adds v to the "rrtype" field.
func (u *DnsRRUpsertBulk) AddRrtype(v uint16) *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.AddRrtype(v)
	})
}

// UpdateRrtype sets the "rrtype" field to the value that was provided on create.
func (u *DnsRRUpsertBulk) UpdateRrtype() *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.UpdateRrtype()
	})
}

// SetRrdata sets the "rrdata" field.
func (u *DnsRRUpsertBulk) SetRrdata(v string) *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.SetRrdata(v)
	})
}

// UpdateRrdata sets the "rrdata" field to the value that was provided on create.
func (u *DnsRRUpsertBulk) UpdateRrdata() *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.UpdateRrdata()
	})
}

// SetClass sets the "class" field.
func (u *DnsRRUpsertBulk) SetClass(v uint16) *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.SetClass(v)
	})
}

// AddClass adds v to the "class" field.
func (u *DnsRRUpsertBulk) AddClass(v uint16) *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.AddClass(v)
	})
}

// UpdateClass sets the "class" field to the value that was provided on create.
func (u *DnsRRUpsertBulk) UpdateClass() *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.UpdateClass()
	})
}

// SetTTL sets the "ttl" field.
func (u *DnsRRUpsertBulk) SetTTL(v uint32) *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.SetTTL(v)
	})
}

// AddTTL adds v to the "ttl" field.
func (u *DnsRRUpsertBulk) AddTTL(v uint32) *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.AddTTL(v)
	})
}

// UpdateTTL sets the "ttl" field to the value that was provided on create.
func (u *DnsRRUpsertBulk) UpdateTTL() *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.UpdateTTL()
	})
}

// SetActivated sets the "activated" field.
func (u *DnsRRUpsertBulk) SetActivated(v bool) *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.SetActivated(v)
	})
}

// UpdateActivated sets the "activated" field to the value that was provided on create.
func (u *DnsRRUpsertBulk) UpdateActivated() *DnsRRUpsertBulk {
	return u.Update(func(s *DnsRRUpsert) {
		s.UpdateActivated()
	})
}

// Exec executes the query.
func (u *DnsRRUpsertBulk) Exec(ctx context.Context) error {
	for i, b := range u.create.builders {
		if len(b.conflict) != 0 {
			return fmt.Errorf("ent: OnConflict was set for builder %d. Set it on the DnsRRCreateBulk instead", i)
		}
	}
	if len(u.create.conflict) == 0 {
		return errors.New("ent: missing options for DnsRRCreateBulk.OnConflict")
	}
	return u.create.Exec(ctx)
}

// ExecX is like Exec, but panics if an error occurs.
func (u *DnsRRUpsertBulk) ExecX(ctx context.Context) {
	if err := u.create.Exec(ctx); err != nil {
		panic(err)
	}
}