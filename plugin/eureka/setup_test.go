package eureka

import (
	"testing"

	"github.com/caddyserver/caddy"
)

func TestSetupEureka(t *testing.T) {
	f = func(baseUrl string) clientAPI {
		return fakeEureka{}
	}
	tests := []struct {
		body          string
		expectedError bool
	}{
		{`eureka`, true},
		{`eureka :`, true},
		{`eureka example.org`, true},
		{`eureka example.org {
	base_url
}`, true},
		{`eureka example.org {
	base_url http://eureka.com:7001
}`, true},
		{`eureka example.org {
	base_url http://eureka.com:7001
	mode
}`, true},
		{`eureka example.org {
	base_url http://eureka.com:7001
	mode test
}`, true},
		{`eureka example.org {
	base_url http://eureka.com:7001
	mode vip
	ttl invalid
}`, true},
		{`eureka example.org {
	base_url http://eureka.com:7001
	mode vip
	ttl 60
}`, false},
		{`eureka example.org {
	base_url http://eureka.com:7001
	mode vip
}`, false},
	}

	for _, test := range tests {
		c := caddy.NewTestController("dns", test.body)
		if err := setup(c); (err == nil) == test.expectedError {
			t.Errorf("Unexpected errors: %v", err)
		}
	}
}
