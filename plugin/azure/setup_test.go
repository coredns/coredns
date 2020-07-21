package azure

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		body          string
		expectedError bool
	}{
		{`azure`, false},
		{`azure :`, true},
		{`azure resource_set:zone`, false},
		{`azure resource_set:zone {
    tenant
}`, true},
		{`azure resource_set:zone {
    tenant abc
}`, false},
		{`azure resource_set:zone {
    client
}`, true},
		{`azure resource_set:zone {
    client abc
}`, false},
		{`azure resource_set:zone {
    subscription
}`, true},
		{`azure resource_set:zone {
    subscription abc
}`, false},
		{`azure resource_set:zone {
    foo
}`, true},
		{`azure resource_set:zone {
    tenant tenant_id
    client client_id
    secret client_secret
    subscription subscription_id
    access public
}`, false},
		{`azure resource_set:zone {
    fallthrough
}`, false},
		{`azure resource_set:zone {
		environment AZUREPUBLICCLOUD
	}`, false},
		{`azure resource_set:zone resource_set:zone {
			fallthrough
		}`, true},
		{`azure resource_set:zone,zone2 {
			access private
		}`, false},
		{`azure resource-set:zone {
			access public
		}`, false},
		{`azure resource-set:zone {
			access foo
		}`, true},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.body)
		if _, _, _, _, err := parse(c); (err == nil) == test.expectedError {
			t.Fatalf("Unexpected errors: %v in test: %d\n\t%s", err, i, test.body)
		}
	}
}
