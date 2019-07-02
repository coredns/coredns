package azure

import (
	"testing"

	"github.com/mholt/caddy"
)

func TestSetupRoute53(t *testing.T) {
	tests := []struct {
		body          string
		expectedError bool
	}{
		{`azure`, false},
		{`azure :`, true},
		{`azure resource-set:hosted-zone`, false},
		{`azure resource-set:hosted-zone {
    azure_tenant_id
}`, true},
		{`azure resource-set:hosted-zone {
    azure_tenant_id
}`, true},
		{`azure resource-set:hosted-zone {
    azure_client_id
}`, true},
		{`azure resource-set:hosted-zone {
    azure_client_secret
}`, true},
		{`azure resource-set:hosted-zone {
    azure_subscription_id
}`, true},
		{`azure resource-set:hosted-zone {
    upstream 10.0.0.1
}`, true},

		{`azure resource-set:hosted-zone {
    upstream
}`, true},
		{`azure resource-set:hosted-zone {
    wat
}`, true},
		{`azure resource-set:hosted-zone {
    azure_tenant_id <azure_tenant_id>
    azure_client_id <azure_client_id>
    azure_client_secret <azure_client_secret>
    azure_subscription_id <azure_subscription_id>
}`, false},

		{`azure resource-set:hosted-zone {
    fallthrough
}`, false},
		{`azure resource-set:hosted-zone {
		azure_auth_location
	}`, true},
		{`azure resource-set:hosted-zone resource-set:hosted-zone {
			fallthrough
		}`, true},
		{`azure resource-set:hosted-zone,hosted-zone-2 {
			fallthrough
		}`, false},
		{`azure resource-set {
			fallthrough
		}`, true},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.body)
		if _, _, _, err := parseCorefile(c); (err == nil) == test.expectedError {
			t.Errorf("Unexpected errors: %v in test: %d", err, i)
		}
	}
}
