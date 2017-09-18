package file

import (
	"testing"
)

func TestWatcher(t *testing.T) {
	w := newWatch()
	d := "/tmp"

	tests := []struct {
		funcs    []func(string) error
		expected int
	}{
		{[]func(string) error{w.remove}, 0},
		{[]func(string) error{w.add}, 1},
		{[]func(string) error{w.add}, 2},
		{[]func(string) error{w.remove}, 1},
		{[]func(string) error{w.remove}, 0},
		{[]func(string) error{w.remove}, 0},
	}

	for i, tc := range tests {
		for _, f := range tc.funcs {
			f(d)
		}
		if x := w.d[d]; x != tc.expected {
			t.Errorf("Test %d, expected %d, go %d", i, tc.expected, x)
		}
	}
}
