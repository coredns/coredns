package kubernetes

import "testing"

func TestPath(t *testing.T) {
	for _, path := range []string{"mydns", "skydns"} {
		k := Kubernetes{PathPrefix: path}
		result := k.Path("service.staging.skydns.local.")
		if result != "/"+path+"/local/skydns/staging/service" {
			t.Errorf("Failure to get domain's path with prefix: %s", result)
		}
	}
}
