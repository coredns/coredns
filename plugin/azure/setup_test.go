package azure

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		body           string
		expectedError  bool
		expectedAccess map[string]string
	}{
		{`azure`, false, nil},
		{`azure :`, true, nil},
		{`azure resource_set:zone`, false, nil},
		{`azure resource_set:zone {
    tenant
}`, true, nil},
		{`azure resource_set:zone {
    tenant abc
}`, false, nil},
		{`azure resource_set:zone {
    client
}`, true, nil},
		{`azure resource_set:zone {
    client abc
}`, false, nil},
		{`azure resource_set:zone {
    subscription
}`, true, nil},
		{`azure resource_set:zone {
    subscription abc
}`, false, nil},
		{`azure resource_set:zone {
    foo
}`, true, nil},
		{`azure resource_set:zone {
    tenant tenant_id
    client client_id
    secret client_secret
    subscription subscription_id
    access public
}`, false, nil},
		{`azure resource_set:zone {
    fallthrough
}`, false, nil},
		{`azure resource_set:zone {
		environment AZUREPUBLICCLOUD
	}`, false, nil},
		{`azure resource_set:zone resource_set:zone {
			fallthrough
		}`, true, nil},
		{`azure resource_set:zone,zone2 {
			access private
		}`, false, nil},
		{`azure resource-set:zone {
			access public
		}`, false, nil},
		{`azure resource-set:zone {
			access foo
		}`, true, nil},
		{`azure rg:zone1 rg:zone2 {
			access private
		}`, false, map[string]string{"rgzone1": "private", "rgzone2": "private"}},
		{`azure rg:zone1 {
			access private
		}
		azure rg:zone2 {
		}`, false, map[string]string{"rgzone1": "private", "rgzone2": "public"}},
		{`azure rg:zone1 rg:zone2 {
		}`, false, map[string]string{"rgzone1": "public", "rgzone2": "public"}},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.body)
		_, _, accessMap, _, err := parse(c)
		if (err == nil) == test.expectedError {
			t.Fatalf("Unexpected errors: %v in test: %d\n\t%s", err, i, test.body)
		}
		if test.expectedAccess == nil {
			continue
		}
		if len(accessMap) != len(test.expectedAccess) {
			t.Fatalf("Test %d: accessMap size mismatch: got %d (%v), want %d (%v)\n\t%s",
				i, len(accessMap), accessMap, len(test.expectedAccess), test.expectedAccess, test.body)
		}
		for k, want := range test.expectedAccess {
			if got := accessMap[k]; got != want {
				t.Fatalf("Test %d: accessMap[%q] = %q, want %q\n\t%s", i, k, got, want, test.body)
			}
		}
	}
}
