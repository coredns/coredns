package object

import (
	"fmt"
	"testing"

	api "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEndpointsIPStripLeadingZeros(t *testing.T) {
	obj := &api.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "endpoint1", Namespace: "test1"},
		Subsets: []api.EndpointSubset{{
			Addresses: []api.EndpointAddress{{IP: "01.02.03.04"}},
			Ports:     []api.EndpointPort{{Port: 80}}},
		},
	}
	expectedIP := "1.2.3.4"

	got, err := ToEndpoints(obj)
	if err != nil {
		t.Fatalf("get added object failed: %v", err)
	}

	o, ok := got.(*Endpoints)
	if !ok {
		t.Fatal("object in index was incorrect type")
	}
	if fmt.Sprintf("%v", o.Subsets[0].Addresses[0].IP) != fmt.Sprintf("%v", expectedIP) {
		t.Fatalf("expected '%v', got '%v'", expectedIP, o.Subsets[0].Addresses[0].IP)
	}
	if fmt.Sprintf("%v", o.IndexIP) != fmt.Sprintf("%v", []string{expectedIP}) {
		t.Fatalf("expected '%v', got '%v'", []string{expectedIP}, o.IndexIP)
	}
}

func TestEndpointSliceIPStripLeadingZeros(t *testing.T) {
	var port int32 = 80
	var portName string = "http"
	var portProt api.Protocol = "tcp"
	obj := &discovery.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Name: "endpoint1", Namespace: "test1"},
		Endpoints: []discovery.Endpoint{{
			Addresses: []string{"01.02.03.04"},
		}},
		Ports: []discovery.EndpointPort{{Port: &port, Protocol: &portProt, Name: &portName}},
	}
	expectedIP := "1.2.3.4"

	got, err := EndpointSliceToEndpoints(obj)
	if err != nil {
		t.Fatalf("get added object failed: %v", err)
	}

	o, ok := got.(*Endpoints)
	if !ok {
		t.Fatal("object in index was incorrect type")
	}
	if fmt.Sprintf("%v", o.Subsets[0].Addresses[0].IP) != fmt.Sprintf("%v", expectedIP) {
		t.Fatalf("expected '%v', got '%v'", expectedIP, o.Subsets[0].Addresses[0].IP)
	}
	if fmt.Sprintf("%v", o.IndexIP) != fmt.Sprintf("%v", []string{expectedIP}) {
		t.Fatalf("expected '%v', got '%v'", []string{expectedIP}, o.IndexIP)
	}
}
