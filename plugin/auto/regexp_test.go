package auto

import (
	"fmt"
	"testing"
)

func TestRewriteToExpand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in       string
		expected string
	}{
		{in: "", expected: ""},
		{in: "{1}", expected: "${1}"},
		{in: "{1", expected: "${1"},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("test_%d", i), func(t *testing.T) {
			t.Parallel()
			got := rewriteToExpand(tc.in)
			if got != tc.expected {
				t.Errorf("Test %d: Expected error %v, but got %v", i, tc.expected, got)
			}
		})
	}
}
