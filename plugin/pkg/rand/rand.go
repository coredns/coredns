// Package rand is used for concurrency safe random number generator.
package rand

import (
	"math/rand"
	"sync"
)

// Rand is used for concurrency safe random number generator.
type Rand struct {
	m sync.Mutex
	r *rand.Rand
}

// New returns a new Rand from seed.
func New(seed int64) *Rand {
	return &Rand{r: rand.New(rand.NewSource(seed))}
}

// Int returns a non-negative pseudo-random int from the Source in Rand.r.
func (r *Rand) Int() int {
	r.m.Lock()
	v := r.r.Int()
	r.m.Unlock()
	return v
}

// Perm returns, as a slice of n ints, a pseudo-random permutation of the
// integers in the half-open interval [0,n) from the Source in Rand.r.
func (r *Rand) Perm(n int) []int {
	r.m.Lock()
	v := r.r.Perm(n)
	r.m.Unlock()
	return v
}
