package kubernetes

import "testing"

func TestSanitizeLabelValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"valid ASCII", "example.com", "example.com"},
		{"valid UTF-8", "例え.jp", "例え.jp"},
		{"empty string", "", ""},
		{"invalid single byte", "host\xff:443", "host\uFFFD:443"},
		{"consecutive invalid bytes", "\xff\xfe\xfd", "\uFFFD"},
		{"mixed valid and invalid", "ok\xffok\xfeok", "ok\uFFFDok\uFFFDok"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeLabelValue(tc.input)
			if got != tc.expected {
				t.Errorf("sanitizeLabelValue(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}
