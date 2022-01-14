package object

import (
	"fmt"
	"testing"

	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceIPStripLeadingZeros(t *testing.T) {
	obj := &api.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "service1", Namespace: "test1"},
		Spec: api.ServiceSpec{
			ClusterIP:   "01.02.03.04",
			ClusterIPs:  []string{"01.02.03.04"},
			ExternalIPs: []string{"05.06.07.08"},
			Ports:       []api.ServicePort{{Port: 80}},
		},
	}
	expectedClusterIPs := []string{"1.2.3.4"}
	expectedExternalIPs := []string{"5.6.7.8"}

	got, err := ToService(obj)
	if err != nil {
		t.Fatal(err)
	}

	svc, ok := got.(*Service)
	if !ok {
		t.Fatal("object was incorrect type")
	}
	if fmt.Sprintf("%v", svc.ClusterIPs) != fmt.Sprintf("%v", expectedClusterIPs) {
		t.Fatalf("expected '%v', got '%v'", expectedClusterIPs, svc.ClusterIPs)
	}
	if fmt.Sprintf("%v", svc.ExternalIPs) != fmt.Sprintf("%v", expectedExternalIPs) {
		t.Fatalf("expected '%v', got '%v'", expectedExternalIPs, svc.ClusterIPs)
	}
}
