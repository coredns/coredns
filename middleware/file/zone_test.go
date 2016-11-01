package file

import "testing"

func TestNameFromRight(t *testing.T) {
	z := NewZone("example.org.", "stdin")

	tests := []struct {
		in     string
		labels int
		shot   bool
		out    string
	}{
		{"example.org.", 0, false, "example.org."},
	}
}
