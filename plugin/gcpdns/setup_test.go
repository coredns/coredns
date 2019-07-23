package gcpdns

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/caddyserver/caddy"
	"github.com/coredns/coredns/plugin/gcpdns/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

func TestSetupGcpdns(t *testing.T) {
	goodFactory := func(ctx context.Context, opts ...option.ClientOption) (*api.Service, error) {
		return newMock(), nil
	}
	badFactory := func(ctx context.Context, opts ...option.ClientOption) (*api.Service, error) {
		return nil, fmt.Errorf("bad factory")
	}

	saCredentialsFile, err := ioutil.TempFile("", "*.json")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = os.Remove(saCredentialsFile.Name())
	}()

	tests := []struct {
		name          string
		body          string
		expectedError bool
		factory       dnsServiceFactory
		saCredentials string
	}{
		{"bad-factory", `gcpdns`, true, badFactory, ""},
		{"simplest", `gcpdns`, false, goodFactory, ""},
		{"name-zone-missing-k-v", `gcpdns :`, true, goodFactory, ""},
		{"vanilla-name-zone", `gcpdns example.org:my-project/org-zone`, false, goodFactory, ""},
		{"invalid-name-zone", `gcpdns example.org:invalid-zone`, true, goodFactory, ""},
		{"name-zone-missing-project", `gcpdns example.org:/org-zone`, true, goodFactory, ""},
		{"name-zone-missing-zone", `gcpdns example.org:my-project/`, true, goodFactory, ""},
		{"vanilla-name-zone_unknown-zone", `gcpdns example.org:my-project/unknown-zone`, true, goodFactory, ""},
		{"vanilla-name-zone_unknown-project", `gcpdns example.org:unknown-project/org-zone`, true, goodFactory, ""},
		{"gcp_service_account_json", `gcpdns example.org:my-project/org-zone {
    gcp_service_account_json DNS_SERVICE_ACCOUNT
}`, false, goodFactory, "eyJ0eXBlIjogInNlcnZpY2VfYWNjb3VudCJ9"},
		{"gcp_service_account_json-bad-json", `gcpdns example.org:my-project/org-zone {
    gcp_service_account_json DNS_SERVICE_ACCOUNT
}`, true, goodFactory, "not base 64"},
		{"gcp_service_account_file", fmt.Sprintf(`gcpdns example.org:my-project/org-zone {
    gcp_service_account_file %s
}`, saCredentialsFile.Name()), false, goodFactory, ""},
		{"gcp_service_account_json-missing-env-name", `gcpdns example.org:my-project/org-zone {
    gcp_service_account_json
}`, true, goodFactory, ""},
		{"gcp_service_account_file-missing-saCredentialsFile-name", `gcpdns example.org:my-project/org-zone {
    gcp_service_account_file
}`, true, goodFactory, ""},
		{"gcp_service_account_json-missing-env", `gcpdns example.org:my-project/org-zone {
    gcp_service_account_json MISSING_ENV
}`, true, goodFactory, ""},
		{"gcp_service_account_file-no-saCredentialsFile", `gcpdns example.org:my-project/org-zone {
    gcp_service_account_file /no/such/saCredentialsFile
}`, true, goodFactory, ""},
		{"double-credentials-i", `gcpdns example.org:my-project/org-zone {
    gcp_service_account_file /etc/coredns/gcp-service-account.json
    gcp_service_account_json DNS_SERVICE_ACCOUNT
}`, true, goodFactory, "eyJ0eXBlIjogInNlcnZpY2VfYWNjb3VudCJ9"},
		{"double-credentials-ii", `gcpdns example.org:my-project/org-zone {
    gcp_service_account_json DNS_SERVICE_ACCOUNT
    gcp_service_account_file /etc/coredns/gcp-service-account.json
}`, true, goodFactory, "eyJ0eXBlIjogInNlcnZpY2VfYWNjb3VudCJ9"},
		{"upstream-with-ip", `gcpdns example.org:my-project/org-zone {
    upstream 10.0.0.1
}`, false, goodFactory, ""},

		{"multiple", fmt.Sprintf(`gcpdns example.org:my-project/org-zone {
    gcp_service_account_json DNS_SERVICE_ACCOUNT
}

gcpdns example.org:another-project/org-zone {
    gcp_service_account_file %s
}`, saCredentialsFile.Name()), false, goodFactory, "eyJ0eXBlIjogInNlcnZpY2VfYWNjb3VudCJ9"},

		{"upstream", `gcpdns example.org:my-project/org-zone {
    upstream
}`, false, goodFactory, ""},
		{"invalid-block", `gcpdns example.org:my-project/org-zone {
    wat
}`, true, goodFactory, ""},
		{"upstream-with-gcp_service_account_json", `gcpdns example.org:my-project/org-zone {
    gcp_service_account_json DNS_SERVICE_ACCOUNT
    upstream 1.2.3.4
}`, false, goodFactory, "eyJ0eXBlIjogInNlcnZpY2VfYWNjb3VudCJ9"},

		{"fallthrough", `gcpdns example.org:my-project/org-zone {
    fallthrough
}`, false, goodFactory, ""},
		{"test-multiple-zones", `gcpdns example.org:my-project/org-zone example.org:my-project/org-zone {
    upstream 1.2.3.4
}`, true, goodFactory, ""},

		{"test", `gcpdns example.org {
    upstream 1.2.3.4
}`, true, goodFactory, ""},
	}

	assert := require.New(t)

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			if tc.saCredentials != "" {
				_ = os.Setenv("DNS_SERVICE_ACCOUNT", tc.saCredentials)
			}
			c := caddy.NewTestController("dns", tc.body)
			err := setup(c, tc.factory)
			if tc.expectedError {
				assert.Error(err)
			} else {
				assert.NoError(err)
			}
		})
	}
}
