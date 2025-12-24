package rand

import (
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		seed int64
	}{
		{
			name: "positive seed",
			seed: 12345,
		},
		{
			name: "zero seed",
			seed: 0,
		},
		{
			name: "negative seed",
			seed: -12345,
		},
		{
			name: "current time seed",
			seed: time.Now().UnixNano(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := New(test.seed)
			if r.r == nil {
				t.Error("internal rand.Rand is nil")
			}
		})
	}
}

func TestIntDeterministic(t *testing.T) {
	seed := int64(42)

	// Create two generators with the same seed
	r1 := New(seed)
	r2 := New(seed)

	// They should produce the same sequence
	for i := range 10 {
		val1 := r1.Int()
		val2 := r2.Int()
		if val1 != val2 {
			t.Errorf("generators with same seed produced different values at iteration %d: %d != %d", i, val1, val2)
		}
	}
}

func TestPerm(t *testing.T) {
	r := New(12345)

	tests := []struct {
		name string
		n    int
	}{
		{
			name: "empty permutation",
			n:    0,
		},
		{
			name: "single element",
			n:    1,
		},
		{
			name: "small permutation",
			n:    5,
		},
		{
			name: "larger permutation",
			n:    20,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			perm := r.Perm(test.n)
			if len(perm) != test.n {
				t.Errorf("Perm(%d) returned slice of length %d", test.n, len(perm))
			}
		})
	}
}

func TestPermDeterministic(t *testing.T) {
	seed := int64(42)
	n := 10

	// Create two generators with the same seed
	r1 := New(seed)
	r2 := New(seed)

	perm1 := r1.Perm(n)
	perm2 := r2.Perm(n)

	// They should produce the same permutation
	if len(perm1) != len(perm2) {
		t.Errorf("permutations have different lengths: %d != %d", len(perm1), len(perm2))
	}

	for i := 0; i < len(perm1) && i < len(perm2); i++ {
		if perm1[i] != perm2[i] {
			t.Errorf("permutations differ at index %d: %d != %d", i, perm1[i], perm2[i])
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	r := New(12345)
	numGoroutines := 10
	numOperations := 100

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numOperations)

	// Test concurrent Int() calls
	for range numGoroutines {
		wg.Go(func() {
			for range numOperations {
				val := r.Int()
				if val < 0 {
					errors <- nil
				}
			}
		})
	}

	// Test concurrent Perm() calls
	for range numGoroutines {
		wg.Go(func() {
			for range numOperations {
				perm := r.Perm(5)
				if len(perm) != 5 {
					errors <- nil
				}
			}
		})
	}

	wg.Wait()
	close(errors)

	errorCount := 0
	for range errors {
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("concurrent access resulted in %d errors", errorCount)
	}
}

func TestConcurrentMixedOperations(t *testing.T) {
	r := New(time.Now().UnixNano())
	numGoroutines := 5
	numOperations := 50

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numOperations)

	// Mix of Int() and Perm() operations running concurrently
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range numOperations {
				if j%2 == 0 {
					val := r.Int()
					if val < 0 {
						errors <- nil
					}
				} else {
					perm := r.Perm(3)
					if len(perm) != 3 {
						errors <- nil
					}
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	errorCount := 0
	for range errors {
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("concurrent mixed operations resulted in %d errors", errorCount)
	}
}
