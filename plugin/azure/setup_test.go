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
    tenant_id
}`, true},
		{`azure resource-set:hosted-zone {
    tenant_id
}`, true},
		{`azure resource-set:hosted-zone {
    client_id
}`, true},
		{`azure resource-set:hosted-zone {
    client_secret
}`, true},
		{`azure resource-set:hosted-zone {
    subscription_id
}`, true},
		{`azure resource-set:hosted-zone {
    upstream 10.0.0.1
}`, true},

		{`azure resource-set:hosted-zone {
    upstream
}`, true},
		{`azure resource-set:hosted-zone {
    foobar
}`, true},
		{`azure resource-set:hosted-zone {
    tenant_id <tenant_id>
    client_id <client_id>
    client_secret <client_secret>
    subscription_id <subscription_id>
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
