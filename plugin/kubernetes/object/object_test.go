package object

import (
	"testing"
)

func TestCopyAndStripLeadingZerosIPv4(t *testing.T) {
	in := []string{
		"010.00.0.01",
		"010.00.0.0000001",
		"10.0.0.2",
		"1.2.3.4",
		"1:2:3::04",
	}
	expect := []string{
		"10.0.0.1",
		"10.0.0.1",
		"10.0.0.2",
		"1.2.3.4",
		"1:2:3::04",
	}
	out := make([]string, len(in))

	n := copyAndStripLeadingZerosIPv4(out, in)

	if n != len(in) {
		t.Errorf("Expected return value %v, got %v", len(in), n)
	}

	for i := range expect {
		if out[i] == expect[i] {
			continue
		}
		t.Errorf("For entry %v expected %v, got %v", i, expect[i], out[i])
	}
}
