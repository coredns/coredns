package object

import (
	"reflect"
	"testing"

	api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	mcs "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

// deepCopyCases holds one fully populated value of every type in this package that
// implements runtime.Object. Every exported field must be set to a non-zero value:
// assertAllFieldsSet enforces that, so a field added to one of these types fails here
// until it is set, which in turn makes TestDeepCopyObjectCopiesEveryField cover it.
func deepCopyCases() []struct {
	name string
	obj  runtime.Object
} {
	endpoints := &Endpoints{
		Version:   "1",
		Name:      "svc1-slice1",
		Namespace: "testns",
		Index:     EndpointsKey("svc1", "testns"),
		IndexIP:   []string{"172.0.0.1"},
		Subsets: []EndpointSubset{{
			Addresses: []EndpointAddress{{
				IP:            "172.0.0.1",
				Hostname:      "ep1a",
				NodeName:      "node1",
				TargetRefName: "pod1",
			}},
			Ports: []EndpointPort{{Port: 80, Name: "http", Protocol: "tcp"}},
		}},
		Zones: map[string]string{"172.0.0.1": "us-east-1a"},
	}

	return []struct {
		name string
		obj  runtime.Object
	}{
		{"Pod", &Pod{
			Version:   "1",
			PodIP:     "10.244.0.1",
			Name:      "pod1",
			Namespace: "testns",
			Labels:    map[string]string{"app": "nginx", "tier": "frontend"},
		}},
		{"Endpoints", endpoints},
		{"MultiClusterEndpoints", &MultiClusterEndpoints{
			Endpoints: *endpoints,
			ClusterId: "cluster1",
		}},
		{"Service", &Service{
			Version:      "1",
			Name:         "svc1",
			Namespace:    "testns",
			Index:        ServiceKey("svc1", "testns"),
			ClusterIPs:   []string{"10.0.0.1"},
			Type:         api.ServiceTypeClusterIP,
			ExternalName: "coredns.io",
			Ports:        []api.ServicePort{{Name: "http", Protocol: api.ProtocolTCP, Port: 80}},
			ExternalIPs:  []string{"1.2.3.4"},
		}},
		{"ServiceImport", &ServiceImport{
			Version:    "1",
			Name:       "svc1",
			Namespace:  "testns",
			Index:      ServiceImportKey("svc1", "testns"),
			ClusterIPs: []string{"10.0.0.1"},
			Type:       mcs.ClusterSetIP,
			Ports:      []mcs.ServicePort{{Name: "http", Protocol: api.ProtocolTCP, Port: 80}},
		}},
		{"Namespace", &Namespace{Version: "1", Name: "testns"}},
	}
}

// assertAllFieldsSet reports any exported field left at its zero value. The embedded
// *Empty carries no data, so it is skipped.
func assertAllFieldsSet(t *testing.T, name string, obj any) {
	t.Helper()
	v := reflect.ValueOf(obj).Elem()
	typ := v.Type()
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() || f.Type == reflect.TypeFor[*Empty]() {
			continue
		}
		if v.Field(i).IsZero() {
			t.Errorf("%s: test fixture leaves field %q at its zero value; set it so DeepCopyObject is actually checked for it", name, f.Name)
		}
	}
}

func TestDeepCopyObjectCopiesEveryField(t *testing.T) {
	for _, tc := range deepCopyCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertAllFieldsSet(t, tc.name, tc.obj)

			got := tc.obj.DeepCopyObject()
			if !reflect.DeepEqual(tc.obj, got) {
				t.Errorf("DeepCopyObject() dropped or altered a field\n got: %+v\nwant: %+v", got, tc.obj)
			}
		})
	}
}

// TestDeepCopyObjectIsDeep checks the copy does not alias the original, so mutating
// one cannot be observed through the other.
func TestDeepCopyObjectIsDeep(t *testing.T) {
	pod := &Pod{
		Version:   "1",
		PodIP:     "10.244.0.1",
		Name:      "pod1",
		Namespace: "testns",
		Labels:    map[string]string{"app": "nginx"},
	}
	cp := pod.DeepCopyObject().(*Pod)
	pod.Labels["app"] = "changed"
	pod.Labels["added"] = "yes"
	if cp.Labels["app"] != "nginx" {
		t.Errorf("copy aliases the original label map: got %q, want %q", cp.Labels["app"], "nginx")
	}
	if _, ok := cp.Labels["added"]; ok {
		t.Error("copy aliases the original label map: a key added afterwards is visible in the copy")
	}

	eps := &Endpoints{
		Version: "1", Name: "s", Namespace: "n", Index: "i",
		IndexIP: []string{"1.2.3.4"},
		Subsets: []EndpointSubset{{
			Addresses: []EndpointAddress{{IP: "1.2.3.4"}},
			Ports:     []EndpointPort{{Port: 80}},
		}},
	}
	epsCopy := eps.DeepCopyObject().(*Endpoints)
	eps.IndexIP[0] = "5.6.7.8"
	eps.Subsets[0].Addresses[0].IP = "5.6.7.8"
	if epsCopy.IndexIP[0] != "1.2.3.4" || epsCopy.Subsets[0].Addresses[0].IP != "1.2.3.4" {
		t.Error("Endpoints copy aliases the original")
	}
}

// Nil maps and slices must stay nil rather than becoming empty, so a round trip does
// not change how a value compares.
func TestDeepCopyObjectPreservesNilLabels(t *testing.T) {
	pod := &Pod{Version: "1", PodIP: "10.244.0.1", Name: "pod1", Namespace: "testns"}
	cp := pod.DeepCopyObject().(*Pod)
	if cp.Labels != nil {
		t.Errorf("nil Labels became %#v after a round trip", cp.Labels)
	}
}
