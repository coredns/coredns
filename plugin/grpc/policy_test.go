package grpc

import (
	"testing"
)

func TestRoundRobinEmpty(t *testing.T) {
	t.Parallel()

	r := &roundRobin{}
	got := r.List(nil)
	if len(got) != 0 {
		t.Fatalf("expected length 0, got %d", len(got))
	}
}

func TestRandomEmpty(t *testing.T) {
	t.Parallel()

	r := &random{}
	got := r.List(nil)
	if len(got) != 0 {
		t.Fatalf("expected length 0, got %d", len(got))
	}
}

func TestSequentialEmpty(t *testing.T) {
	t.Parallel()

	r := &sequential{}
	got := r.List(nil)
	if len(got) != 0 {
		t.Fatalf("expected length 0, got %d", len(got))
	}
}

func TestPoliciesOrdering(t *testing.T) {
	t.Parallel()

	p0 := &Proxy{addr: "p0"}
	p1 := &Proxy{addr: "p1"}
	p2 := &Proxy{addr: "p2"}
	in := []*Proxy{p0, p1, p2}

	t.Run("sequential keeps order", func(t *testing.T) {
		t.Parallel()

		r := &sequential{}
		got := r.List(in)
		if len(got) != len(in) {
			t.Fatalf("expected length %d, got %d", len(in), len(got))
		}
		for i := range in {
			if got[i] != in[i] {
				t.Fatalf("sequential order changed at %d: want %p, got %p", i, in[i], got[i])
			}
		}
	})

	t.Run("round robin advances and permutation", func(t *testing.T) {
		t.Parallel()

		r := &roundRobin{}

		got1 := r.List(in)
		if !isPermutation(in, got1) {
			t.Fatalf("first call: expected permutation of input")
		}
		if got1[0] != p1 {
			t.Fatalf("first element should advance to p1, got %p", got1[0])
		}

		got2 := r.List(in)
		if !isPermutation(in, got2) {
			t.Fatalf("second call: expected permutation of input")
		}
		if got2[0] != p2 {
			t.Fatalf("first element should advance to p2 on second call, got %p", got2[0])
		}

		got3 := r.List(in)
		if !isPermutation(in, got3) {
			t.Fatalf("third call: expected permutation of input")
		}
		if got3[0] != p0 {
			t.Fatalf("first element should wrap to p0 on third call, got %p", got3[0])
		}
	})

	t.Run("random is a permutation", func(t *testing.T) {
		t.Parallel()

		r := &random{}
		got := r.List(in)
		if !isPermutation(in, got) {
			t.Fatalf("random did not return a permutation of input")
		}
	})

	t.Run("random with two proxies", func(t *testing.T) {
		t.Parallel()

		r := &random{}
		in2 := []*Proxy{p0, p1}
		got := r.List(in2)
		if !isPermutation(in2, got) {
			t.Fatalf("random did not return a permutation of input")
		}
	})
}

// Helper: returns true if b is a permutation of a (same multiset of pointers).
func isPermutation(a, b []*Proxy) bool {
	if len(a) != len(b) {
		return false
	}
	count := make(map[*Proxy]int, len(a))
	for _, p := range a {
		count[p]++
	}
	for _, p := range b {
		count[p]--
		if count[p] < 0 {
			return false
		}
	}
	return true
}
