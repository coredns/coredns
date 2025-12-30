package clouddns

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/coredns/caddy"

	"google.golang.org/api/option"
)

func TestSetupCloudDNS(t *testing.T) {
	f = func(ctx context.Context, opt option.ClientOption) (gcpDNS, error) {
		return fakeGCPClient{}, nil
	}

	validCreds := filepath.Join(t.TempDir(), "valid_creds.json")
	if err := os.WriteFile(validCreds, []byte(`{"type": "service_account"}`), 0644); err != nil {
		t.Fatalf("Failed to create valid creds: %v", err)
	}

	invalidTypeCreds := filepath.Join(t.TempDir(), "invalid_type_creds.json")
	if err := os.WriteFile(invalidTypeCreds, []byte(`{"type": "bad_type"}`), 0644); err != nil {
		t.Fatalf("Failed to create invalid creds: %v", err)
	}

	emptyCreds := filepath.Join(t.TempDir(), "empty_creds.json")
	if err := os.WriteFile(emptyCreds, []byte(`{}`), 0644); err != nil {
		t.Fatalf("Failed to create empty creds: %v", err)
	}

	invalidJSONCreds := filepath.Join(t.TempDir(), "invalid_json_creds.json")
	if err := os.WriteFile(invalidJSONCreds, []byte(`{`), 0644); err != nil {
		t.Fatalf("Failed to create invalid JSON creds: %v", err)
	}

	tests := []struct {
		body          string
		expectedError bool
	}{
		{`clouddns`, false},
		{`clouddns :`, true},
		{`clouddns ::`, true},
		{`clouddns example.org.:example-project:zone-name`, false},
		{`clouddns example.org.:example-project:zone-name { }`, false},
		{`clouddns example.org.:example-project: { }`, true},
		{`clouddns example.org.:example-project:zone-name { }`, false},
		{`clouddns example.org.:example-project:zone-name { wat
}`, true},
		{`clouddns example.org.:example-project:zone-name {
    fallthrough
}`, false},
		{`clouddns example.org.:example-project:zone-name {
    credentials
}`, true},
		{`clouddns example.org.:example-project:zone-name example.org.:example-project:zone-name {
	}`, true},

		{`clouddns example.org {
	}`, true},
		{fmt.Sprintf(`clouddns example.org.:example-project:zone-name {
    credentials %s
}`, validCreds), false},
		{fmt.Sprintf(`clouddns example.org.:example-project:zone-name {
    credentials %s
}`, invalidTypeCreds), true},
		{fmt.Sprintf(`clouddns example.org.:example-project:zone-name {
    credentials %s
}`, emptyCreds), true},
		{fmt.Sprintf(`clouddns example.org.:example-project:zone-name {
    credentials %s
}`, invalidJSONCreds), true},
	}

	for _, test := range tests {
		c := caddy.NewTestController("dns", test.body)
		if err := setup(c); (err == nil) == test.expectedError {
			t.Errorf("Unexpected errors: %v", err)
		}
	}
}
