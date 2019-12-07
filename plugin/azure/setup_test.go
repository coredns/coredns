package azure

import (
	"testing"

	"github.com/caddyserver/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		body          string
		expectedError bool
	}{
		{`azure`, false},
		{`azure :`, true},
		{`azure resource_set:zone`, true},
		{`azure resource_set:zone:public`, false},
		{`azure resource_set:zone:private`, false},
		{`azure resource_set:zone:foo`, true},
		{`azure resource_set:zone:public {
    tenant
}`, true},
		{`azure resource_set:zone:public {
    tenant
}`, true},
		{`azure resource_set:zone:public {
    client
}`, true},
		{`azure resource_set:zone:public {
    secret
}`, true},
		{`azure resource_set:zone:public {
    subscription
}`, true},
		{`azure resource_set:zone:public {
    upstream 10.0.0.1
}`, true},

		{`azure resource_set:zone:public {
    upstream
}`, true},
		{`azure resource_set:zone:public {
    foobar
}`, true},
		{`azure resource_set:zone:public {
    tenant tenant_id
    client client_id
    secret client_secret
    subscription subscription_id
}`, false},

		{`azure resource_set:zone:public {
    fallthrough
}`, false},
		{`azure resource_set:zone:public {
		environment AZUREPUBLICCLOUD
	}`, false},
		{`azure resource_set:zone:public resource_set:zone:private {
			fallthrough
		}`, true},
		{`azure resource_set:zone,zone2:public {
			fallthrough
		}`, false},
		{`azure resource-set {
			fallthrough
		}`, true},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.body)
		if _, _, _, err := parse(c); (err == nil) == test.expectedError {
			t.Fatalf("Unexpected errors: %v in test: %d\n\t%s", err, i, test.body)
		}
	}
}
