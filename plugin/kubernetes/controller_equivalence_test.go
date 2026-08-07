package kubernetes

import (
	"testing"

	"github.com/coredns/coredns/plugin/kubernetes/object"

	mcs "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

// TestSubsetsEquivalent checks that subsetsEquivalent only treats two subsets
// as equal when their addresses and ports match exactly.
func TestSubsetsEquivalent(t *testing.T) {
	base := object.EndpointSubset{
		Addresses: []object.EndpointAddress{{IP: "10.0.0.1", Hostname: "a"}},
		Ports:     []object.EndpointPort{{Name: "http", Port: 80, Protocol: "TCP"}},
	}

	tests := []struct {
		name string
		a    object.EndpointSubset
		b    object.EndpointSubset
		want bool
	}{
		{"identical", base, base, true},
		{"different address count", base, object.EndpointSubset{
			Addresses: []object.EndpointAddress{},
			Ports:     base.Ports,
		}, false},
		{"different port count", base, object.EndpointSubset{
			Addresses: base.Addresses,
			Ports:     []object.EndpointPort{},
		}, false},
		{"different ip", base, object.EndpointSubset{
			Addresses: []object.EndpointAddress{{IP: "10.0.0.2", Hostname: "a"}},
			Ports:     base.Ports,
		}, false},
		{"different hostname", base, object.EndpointSubset{
			Addresses: []object.EndpointAddress{{IP: "10.0.0.1", Hostname: "b"}},
			Ports:     base.Ports,
		}, false},
		{"different port number", base, object.EndpointSubset{
			Addresses: base.Addresses,
			Ports:     []object.EndpointPort{{Name: "http", Port: 8080, Protocol: "TCP"}},
		}, false},
		{"different port protocol", base, object.EndpointSubset{
			Addresses: base.Addresses,
			Ports:     []object.EndpointPort{{Name: "http", Port: 80, Protocol: "UDP"}},
		}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := subsetsEquivalent(tc.a, tc.b); got != tc.want {
				t.Errorf("subsetsEquivalent() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestEndpointsEquivalent checks endpointsEquivalent's nil handling and its
// comparison of subsets across two Endpoints objects.
func TestEndpointsEquivalent(t *testing.T) {
	subset := object.EndpointSubset{
		Addresses: []object.EndpointAddress{{IP: "10.0.0.1"}},
		Ports:     []object.EndpointPort{{Name: "http", Port: 80, Protocol: "TCP"}},
	}
	a := &object.Endpoints{Subsets: []object.EndpointSubset{subset}}
	b := &object.Endpoints{Subsets: []object.EndpointSubset{subset}}

	if !endpointsEquivalent(a, b) {
		t.Error("expected equivalent endpoints to be equivalent")
	}

	if endpointsEquivalent(nil, b) {
		t.Error("expected nil endpoints to not be equivalent")
	}
	if endpointsEquivalent(a, nil) {
		t.Error("expected nil endpoints to not be equivalent")
	}

	c := &object.Endpoints{Subsets: []object.EndpointSubset{subset, subset}}
	if endpointsEquivalent(a, c) {
		t.Error("expected endpoints with different subset counts to not be equivalent")
	}

	d := &object.Endpoints{Subsets: []object.EndpointSubset{{
		Addresses: []object.EndpointAddress{{IP: "10.0.0.2"}},
		Ports:     subset.Ports,
	}}}
	if endpointsEquivalent(a, d) {
		t.Error("expected endpoints with different addresses to not be equivalent")
	}
}

// TestMulticlusterEndpointsEquivalent checks that multiclusterEndpointsEquivalent
// also accounts for ClusterId on top of the embedded Endpoints comparison.
func TestMulticlusterEndpointsEquivalent(t *testing.T) {
	ep := object.Endpoints{Subsets: []object.EndpointSubset{{
		Addresses: []object.EndpointAddress{{IP: "10.0.0.1"}},
		Ports:     []object.EndpointPort{{Name: "http", Port: 80, Protocol: "TCP"}},
	}}}

	a := &object.MultiClusterEndpoints{Endpoints: ep, ClusterId: "cluster1"}
	b := &object.MultiClusterEndpoints{Endpoints: ep, ClusterId: "cluster1"}
	if !multiclusterEndpointsEquivalent(a, b) {
		t.Error("expected equivalent multicluster endpoints to be equivalent")
	}

	c := &object.MultiClusterEndpoints{Endpoints: ep, ClusterId: "cluster2"}
	if multiclusterEndpointsEquivalent(a, c) {
		t.Error("expected different cluster ids to not be equivalent")
	}

	if multiclusterEndpointsEquivalent(nil, b) {
		t.Error("expected nil multicluster endpoints to not be equivalent")
	}
}

// TestServiceImportEquivalent checks serviceImportEquivalent's add/remove
// handling and its comparison of type and ports.
func TestServiceImportEquivalent(t *testing.T) {
	svc := &object.ServiceImport{
		Type:  mcs.ClusterSetIP,
		Ports: []mcs.ServicePort{{Name: "http", Port: 80, Protocol: "TCP"}},
	}
	svcCopy := &object.ServiceImport{
		Type:  svc.Type,
		Ports: svc.Ports,
	}

	if !serviceImportEquivalent(svc, svcCopy) {
		t.Error("expected equivalent service imports to be equivalent")
	}

	if serviceImportEquivalent(svc, nil) {
		t.Error("expected added/removed service import to not be equivalent")
	}
	if serviceImportEquivalent(nil, svc) {
		t.Error("expected added/removed service import to not be equivalent")
	}

	diffType := &object.ServiceImport{Type: mcs.Headless, Ports: svc.Ports}
	if serviceImportEquivalent(svc, diffType) {
		t.Error("expected different types to not be equivalent")
	}

	diffPorts := &object.ServiceImport{
		Type:  svc.Type,
		Ports: []mcs.ServicePort{{Name: "http", Port: 8080, Protocol: "TCP"}},
	}
	if serviceImportEquivalent(svc, diffPorts) {
		t.Error("expected different ports to not be equivalent")
	}
}
