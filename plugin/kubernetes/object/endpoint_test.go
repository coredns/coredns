package object

import (
	"testing"

	discovery "k8s.io/api/discovery/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEndpointSliceToEndpointsAddressTypes(t *testing.T) {
	ready := true
	tests := []struct {
		name      string
		addrType  discovery.AddressType
		addresses []string
		want      []string
	}{
		{"ipv4", discovery.AddressTypeIPv4, []string{"172.0.0.1", "172.0.0.2"}, []string{"172.0.0.1", "172.0.0.2"}},
		{"ipv4 slices drop ipv6 addresses", discovery.AddressTypeIPv4, []string{"172.0.0.1", "fd00::1"}, []string{"172.0.0.1"}},
		{"ipv6", discovery.AddressTypeIPv6, []string{"fd00::1"}, []string{"fd00::1"}},
		{"ipv6 slices drop ipv4 addresses", discovery.AddressTypeIPv6, []string{"fd00::1", "172.0.0.1"}, []string{"fd00::1"}},
		{"fqdn slices publish no addresses", discovery.AddressTypeFQDN, []string{"other.ns.svc.cluster.local", "db.example.com"}, nil},
		{"unknown address types publish no addresses", discovery.AddressType("Future"), []string{"172.0.0.1"}, nil},
		{"non-ip address in an ip slice is dropped", discovery.AddressTypeIPv4, []string{"172.0.0.1", "not-an-ip"}, []string{"172.0.0.1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			slice := &discovery.EndpointSlice{
				ObjectMeta:  meta.ObjectMeta{Name: "svc1-abcde", Namespace: "testns", Labels: map[string]string{discovery.LabelServiceName: "svc1"}},
				AddressType: tc.addrType,
				Ports:       []discovery.EndpointPort{{}},
			}
			for _, a := range tc.addresses {
				slice.Endpoints = append(slice.Endpoints, discovery.Endpoint{Addresses: []string{a}, Conditions: discovery.EndpointConditions{Ready: &ready}})
			}
			for _, convert := range []ToFunc{EndpointSliceToEndpoints, EndpointSliceToEndpointsWithZones} {
				obj, err := convert(slice.DeepCopy())
				if err != nil {
					t.Fatalf("conversion failed: %v", err)
				}
				e := obj.(*Endpoints)
				if e.Index != EndpointsKey("svc1", "testns") {
					t.Errorf("Index = %q, want %q", e.Index, EndpointsKey("svc1", "testns"))
				}
				var got []string
				for _, s := range e.Subsets {
					for _, a := range s.Addresses {
						got = append(got, a.IP)
					}
				}
				if len(got) != len(tc.want) {
					t.Fatalf("addresses = %v, want %v", got, tc.want)
				}
				for i := range got {
					if got[i] != tc.want[i] {
						t.Errorf("address %d = %q, want %q", i, got[i], tc.want[i])
					}
				}
				if len(e.IndexIP) != len(tc.want) {
					t.Errorf("IndexIP = %v, want %v", e.IndexIP, tc.want)
				}
			}
		})
	}
}
