package route53

import (
	"context"
	"testing"

	"github.com/coredns/caddy"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
)

func TestSetupRoute53(t *testing.T) {
	f = func(_ context.Context, _ []func(*config.LoadOptions) error, _ []func(*route53.Options)) (route53Client, error) {
		return fakeRoute53{}, nil
	}

	tests := []struct {
		body          string
		expectedError bool
	}{
		{`route53`, false},
		{`route53 :`, true},
		{`route53 example.org:12345678`, false},
		{`route53 example.org:12345678 {
    aws_access_key
}`, true},
		{`route53 example.org:12345678 { }`, false},

		{`route53 example.org:12345678 { }`, false},
		{`route53 example.org:12345678 { wat
}`, true},
		{`route53 example.org:12345678 {
    aws_access_key ACCESS_KEY_ID SEKRIT_ACCESS_KEY
}`, false},

		{`route53 example.org:12345678 {
    fallthrough
}`, false},
		{`route53 example.org:12345678 {
		credentials
	}`, true},

		{`route53 example.org:12345678 {
		credentials default
	}`, false},
		{`route53 example.org:12345678 {
		credentials default credentials
	}`, false},
		{`route53 example.org:12345678 {
		credentials default credentials extra-arg
	}`, true},
		{`route53 example.org:12345678 example.org:12345678 {
	}`, true},

		{`route53 example.org:12345678 {
	refresh 90
}`, false},
		{`route53 example.org:12345678 {
	refresh 5m
}`, false},
		{`route53 example.org:12345678 {
	refresh
}`, true},
		{`route53 example.org:12345678 {
	refresh foo
}`, true},
		{`route53 example.org:12345678 {
	refresh -1m
}`, true},

		{`route53 example.org {
	}`, true},
		{`route53 example.org:12345678 {
    aws_endpoint
}`, true},
		{`route53 example.org:12345678 {
    aws_endpoint https://localhost
}`, false},
	}

	for _, test := range tests {
		c := caddy.NewTestController("dns", test.body)
		if err := setup(c); (err == nil) == test.expectedError {
			t.Errorf("Unexpected errors: %v", err)
		}
	}
}
