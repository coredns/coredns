package msg

import "testing"

func TestPath(t *testing.T) {
	for _, path := range []string{"mydns", "skydns"} {
		result := Path("service.staging.skydns.local.", path)
		if result != "/"+path+"/local/skydns/staging/service" {
			t.Errorf("Failure to get domain's path with prefix: %s", result)
		}
	}
}

func TestPath(t *testing.T) {
	result1 := Domain("/skydns/local/cluster/staging/service/")
	if result1 != "service.staging.cluster.local." {
		t.Errorf("==>%s", result1)
	}

	result2 := Domain("/skydns/local/cluster/staging/service")
	if result2 != "service.staging.cluster.local." {
		t.Errorf("==>%s", result2)
	}
}
