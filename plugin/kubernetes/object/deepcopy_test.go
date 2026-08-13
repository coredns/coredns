package object

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	mcs "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func ptrTo[T any](v T) *T { return &v }

// dump renders an object for a failure message. %+v prints an aliased pointer field as
// an address, which hides the value that actually differs, so render as JSON instead.
func dump(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(b)
}

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
			Ports: []api.ServicePort{{
				Name: "http", Protocol: api.ProtocolTCP, Port: 80,
				// A pointer field, so a slice copy alone leaves it shared.
				AppProtocol: ptrTo("kubernetes.io/h2c"),
			}},
			ExternalIPs: []string{"1.2.3.4"},
		}},
		{"ServiceImport", &ServiceImport{
			Version:    "1",
			Name:       "svc1",
			Namespace:  "testns",
			Index:      ServiceImportKey("svc1", "testns"),
			ClusterIPs: []string{"10.0.0.1"},
			Type:       mcs.ClusterSetIP,
			Ports: []mcs.ServicePort{{
				Name: "http", Protocol: api.ProtocolTCP, Port: 80,
				AppProtocol: ptrTo("kubernetes.io/h2c"),
			}},
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
				t.Errorf("DeepCopyObject() dropped or altered a field\n got: %s\nwant: %s", dump(got), dump(tc.obj))
			}
		})
	}
}

// mutateEverything changes every value in v that is reachable through a pointer,
// slice, or map. Anything the copy still shares with the original then shows up as a
// change to the copy. Values reached only by value are changed too; that is harmless,
// because the caller inspects the copy rather than the original.
func mutateEverything(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			mutateEverything(v.Elem())
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			mutateEverything(v.Index(i))
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			// Map values are not addressable, so mutate a copy and write it back.
			e := reflect.New(v.Type().Elem()).Elem()
			e.Set(v.MapIndex(k))
			mutateEverything(e)
			v.SetMapIndex(k, e)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				mutateEverything(v.Field(i))
			}
		}
	case reflect.String:
		if v.CanSet() {
			v.SetString(v.String() + "-mutated")
		}
	case reflect.Bool:
		if v.CanSet() {
			v.SetBool(!v.Bool())
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.CanSet() {
			v.SetInt(v.Int() + 1)
		}
	}
}

// TestDeepCopyObjectIsDeep checks that a copy shares nothing with the original: after
// mutating every reference-reachable value in the original, the copy must still equal
// a pristine fixture. Driving this off deepCopyCases keeps the guarantee package-wide,
// so a type that gains a pointer, slice, or map field is covered here as soon as
// assertAllFieldsSet forces the fixture to populate it.
func TestDeepCopyObjectIsDeep(t *testing.T) {
	for i, tc := range deepCopyCases() {
		t.Run(tc.name, func(t *testing.T) {
			// Rebuild per subtest: the fixtures deliberately share backing arrays
			// (MultiClusterEndpoints is built from the same Endpoints value), so
			// mutating one case would otherwise corrupt another.
			obj := deepCopyCases()[i].obj
			pristine := deepCopyCases()[i].obj

			cp := obj.DeepCopyObject()
			mutateEverything(reflect.ValueOf(obj))

			if !reflect.DeepEqual(cp, pristine) {
				t.Errorf("mutating the original was visible through the copy, so DeepCopyObject aliases it\n got: %s\nwant: %s", dump(cp), dump(pristine))
			}
		})
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
