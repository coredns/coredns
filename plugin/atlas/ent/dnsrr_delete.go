// Code generated by ent, DO NOT EDIT.

package ent

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/schema/field"
	"github.com/coredns/coredns/plugin/atlas/ent/dnsrr"
	"github.com/coredns/coredns/plugin/atlas/ent/predicate"
)

// DnsRRDelete is the builder for deleting a DnsRR entity.
type DnsRRDelete struct {
	config
	hooks    []Hook
	mutation *DnsRRMutation
}

// Where appends a list predicates to the DnsRRDelete builder.
func (drd *DnsRRDelete) Where(ps ...predicate.DnsRR) *DnsRRDelete {
	drd.mutation.Where(ps...)
	return drd
}

// Exec executes the deletion query and returns how many vertices were deleted.
func (drd *DnsRRDelete) Exec(ctx context.Context) (int, error) {
	return withHooks[int, DnsRRMutation](ctx, drd.sqlExec, drd.mutation, drd.hooks)
}

// ExecX is like Exec, but panics if an error occurs.
func (drd *DnsRRDelete) ExecX(ctx context.Context) int {
	n, err := drd.Exec(ctx)
	if err != nil {
		panic(err)
	}
	return n
}

func (drd *DnsRRDelete) sqlExec(ctx context.Context) (int, error) {
	_spec := sqlgraph.NewDeleteSpec(dnsrr.Table, sqlgraph.NewFieldSpec(dnsrr.FieldID, field.TypeInt))
	if ps := drd.mutation.predicates; len(ps) > 0 {
		_spec.Predicate = func(selector *sql.Selector) {
			for i := range ps {
				ps[i](selector)
			}
		}
	}
	affected, err := sqlgraph.DeleteNodes(ctx, drd.driver, _spec)
	if err != nil && sqlgraph.IsConstraintError(err) {
		err = &ConstraintError{msg: err.Error(), wrap: err}
	}
	drd.mutation.done = true
	return affected, err
}

// DnsRRDeleteOne is the builder for deleting a single DnsRR entity.
type DnsRRDeleteOne struct {
	drd *DnsRRDelete
}

// Where appends a list predicates to the DnsRRDelete builder.
func (drdo *DnsRRDeleteOne) Where(ps ...predicate.DnsRR) *DnsRRDeleteOne {
	drdo.drd.mutation.Where(ps...)
	return drdo
}

// Exec executes the deletion query.
func (drdo *DnsRRDeleteOne) Exec(ctx context.Context) error {
	n, err := drdo.drd.Exec(ctx)
	switch {
	case err != nil:
		return err
	case n == 0:
		return &NotFoundError{dnsrr.Label}
	default:
		return nil
	}
}

// ExecX is like Exec, but panics if an error occurs.
func (drdo *DnsRRDeleteOne) ExecX(ctx context.Context) {
	if err := drdo.Exec(ctx); err != nil {
		panic(err)
	}
}
