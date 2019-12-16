package policy

import (
	"math/rand"
	"sync/atomic"
)

// Policy defines a policy we use for selecting upstreams.
type Policy interface {
	List(policy ...interface{}) []interface{}
	String() string
}

// Random is a policy that implements random upstream selection.
type Random struct{}

var _ Policy = &Random{}

// String returns the name of policy Random
func (r *Random) String() string { return "random" }

// List returns a set of proxies to be used for this client depending on Random policy.
func (r *Random) List(p ...interface{}) []interface{} {
	switch len(p) {
	case 1:
		return p
	case 2:
		if rand.Int()%2 == 0 {
			return []interface{}{p[1], p[0]} // swap
		}
		return p
	}

	perms := rand.Perm(len(p))
	rnd := make([]interface{}, len(p))

	for i, p1 := range perms {
		rnd[i] = p[p1]
	}
	return rnd
}

// RoundRobin is a policy that selects hosts based on round robin ordering.
type RoundRobin struct {
	robin uint32
}

var _ Policy = &RoundRobin{}

// String returns the name of policy RoundRobin
func (r *RoundRobin) String() string { return "round_robin" }

// List returns a set of proxies to be used for this client depending on RoundRobin policy.
func (r *RoundRobin) List(p ...interface{}) []interface{} {
	poolLen := uint32(len(p))
	i := atomic.AddUint32(&r.robin, 1) % poolLen

	robin := []interface{}{p[i]}
	robin = append(robin, p[:i]...)
	robin = append(robin, p[i+1:]...)

	return robin
}

// Sequential is a policy that selects hosts based on sequential ordering.
type Sequential struct{}

var _ Policy = &Sequential{}

// String returns the name of policy Sequential
func (r *Sequential) String() string { return "sequential" }

// List returns a set of proxies without filter.
func (r *Sequential) List(p ...interface{}) []interface{} {
	return p
}
