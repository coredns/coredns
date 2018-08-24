package kubernetes

import (
	"testing"

	yaml "gopkg.in/yaml.v2"
)

func TestAuthInfoFromConfig(t *testing.T) {
	kubeconfig := `
    apiVersion: v1
    clusters:
    - cluster:
        server: http://test-server:8080
      name: cluster-without-ca
    - cluster:
        certificate-authority: /var/run/secrets/ca.crt
        server: http://test-server:8080
      name: cluster-with-ca
    contexts:
    - context:
        cluster: cluster-with-ca
        namespace: default
        user: user-tls
      name: context-tls
    - context:
        cluster: cluster-without-ca
        namespace: default
        user: user-username-password
      name: context-username-password
    - context:
        cluster: cluster-without-ca
        namespace: default
        user: user-token
      name: context-token
    kind: Config
    users:
    - name: user-tls
      user:
        client-certificate: /var/run/secrets/client.crt
        client-key: /var/run/secrets/client.key
    - name: user-username-password
      user:
        username: coredns
        password: kubernetes
    - name: user-token
      user:
        token: some-token-123
  `
	config := Config{}
	if err := yaml.Unmarshal([]byte(kubeconfig), &config); err != nil {
		t.Fatalf("unable to load test kubeconfig: %v", err)
	}
	var tests = []struct {
		name          string
		contextName   string
		APICertAuth   string
		APIClientCert string
		APIClientKey  string
		APIUsername   string
		APIPassword   string
		APIToken      string
	}{
		{
			name:          "tls authenticated",
			contextName:   "context-tls",
			APICertAuth:   "/var/run/secrets/ca.crt",
			APIClientCert: "/var/run/secrets/client.crt",
			APIClientKey:  "/var/run/secrets/client.key",
		},
		{
			name:        "username and password",
			contextName: "context-username-password",
			APIUsername: "coredns",
			APIPassword: "kubernetes",
		},
		{
			name:        "token",
			contextName: "context-token",
			APIToken:    "some-token-123",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			k8s := Kubernetes{}
			k8s.AuthInfoFromConfig(config, test.contextName)
			if k8s.APICertAuth != test.APICertAuth {
				t.Errorf("unexpected certificate authority: have %s; expected %s",
					k8s.APICertAuth, test.APICertAuth,
				)
			}
			if k8s.APIClientCert != test.APIClientCert {
				t.Errorf("unexpected client certificate: have %s; expected %s",
					k8s.APIClientCert, test.APIClientCert,
				)
			}
			if k8s.APIClientKey != test.APIClientKey {
				t.Errorf("unexpected client key: have %s; expected %s",
					k8s.APIClientKey, test.APIClientKey,
				)
			}
			if k8s.APIUsername != test.APIUsername {
				t.Errorf("unexpected client username: have %s; want %s",
					k8s.APIUsername, test.APIUsername,
				)
			}
			if k8s.APIPassword != test.APIPassword {
				t.Errorf("unexpected client password: have %s; want %s",
					k8s.APIPassword, test.APIPassword,
				)
			}
			if k8s.APIToken != test.APIToken {
				t.Errorf("unexpected client token: have %s; want %s",
					k8s.APIToken, test.APIToken,
				)
			}
		})
	}
}
