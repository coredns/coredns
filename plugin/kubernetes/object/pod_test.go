package object

import (
	"fmt"
	"testing"

	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodIPStripLeadingZeros(t *testing.T) {
	obj := &api.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "test1"},
		Status: api.PodStatus{
			PodIP: "01.02.03.04",
		},
	}
	expectedIP := "1.2.3.4"

	got, err := ToPod(obj)
	if err != nil {
		t.Fatalf("get added object failed: %v", err)
	}

	o, ok := got.(*Pod)
	if !ok {
		t.Fatal("object in index was incorrect type")
	}
	if fmt.Sprintf("%v", o.PodIP) != fmt.Sprintf("%v", expectedIP) {
		t.Fatalf("expected '%v', got '%v'", expectedIP, o.PodIP)
	}
	if fmt.Sprintf("%v", o.PodIP) != fmt.Sprintf("%v", expectedIP) {
		t.Fatalf("expected '%v', got '%v'", expectedIP, o.PodIP)
	}
}
