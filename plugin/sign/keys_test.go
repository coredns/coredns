package sign

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestKeyParseDirectory(t *testing.T) {
	tests := []struct {
		input       string
		shouldErr   bool
		expectedErr string
		numKeys     int
	}{
		{`sign testdata/db.miek.nl miek.nl {
			key directory testdata
		 }`,
			false, "", 1,
		},
		{`sign testdata/db.miek.nl miek.nl {
			key directory
		 }`,
			true, "Wrong argument count", 0,
		},
		{`sign testdata/db.miek.nl miek.nl {
			key directory testdata/does-not-exist
		 }`,
			false, "", 0,
		},
	}

	for i, tc := range tests {
		c := caddy.NewTestController("dns", tc.input)
		sign, err := parse(c)

		if tc.shouldErr {
			if err == nil {
				t.Fatalf("Test %d: expected error, got none", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Test %d: expected no error, got: %v", i, err)
		}

		keys := sign.signers[0].keys
		if len(keys) != tc.numKeys {
			t.Fatalf("Test %d: expected %d key(s), got %d", i, tc.numKeys, len(keys))
		}
		if tc.numKeys > 0 && keys[0].KeyTag != 59725 {
			t.Errorf("Test %d: expected key tag 59725, got %d", i, keys[0].KeyTag)
		}
	}
}
