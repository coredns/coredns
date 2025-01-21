package object

import (
	"github.com/coredns/coredns/plugin/kubernetes/object"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	mcs "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

// Endpoints is a stripped down api.Endpoints with only the items we need for CoreDNS.
type Endpoints struct {
	object.Endpoints
	ClusterId string
	*object.Empty
}

// EndpointsKey returns a string using for the index.
func EndpointsKey(name, namespace string) string { return name + "." + namespace }

// EndpointSliceToEndpoints converts a *discovery.EndpointSlice to a *Endpoints.
func EndpointSliceToEndpoints(obj meta.Object) (meta.Object, error) {
	ends, err := object.EndpointSliceToEndpoints(obj)
	if err != nil {
		return nil, err
	}
	labels := obj.GetLabels()
	e := &Endpoints{
		Endpoints: *ends.(*object.Endpoints),
		ClusterId: labels[mcs.LabelSourceCluster],
	}
	e.Endpoints.Index = EndpointsKey(labels[mcs.LabelServiceName], ends.GetNamespace())

	return e, nil
}

var _ runtime.Object = &Endpoints{}

// DeepCopyObject implements the ObjectKind interface.
func (e *Endpoints) DeepCopyObject() runtime.Object {
	e1 := &Endpoints{
		ClusterId: e.ClusterId,
		Endpoints: *e.Endpoints.DeepCopyObject().(*object.Endpoints),
	}
	return e1
}

// GetNamespace implements the metav1.Object interface.
func (e *Endpoints) GetNamespace() string { return e.Endpoints.GetNamespace() }

// SetNamespace implements the metav1.Object interface.
func (e *Endpoints) SetNamespace(namespace string) {}

// GetName implements the metav1.Object interface.
func (e *Endpoints) GetName() string { return e.Endpoints.GetName() }

// SetName implements the metav1.Object interface.
func (e *Endpoints) SetName(name string) {}

// GetResourceVersion implements the metav1.Object interface.
func (e *Endpoints) GetResourceVersion() string { return e.Endpoints.GetResourceVersion() }

// SetResourceVersion implements the metav1.Object interface.
func (e *Endpoints) SetResourceVersion(version string) {}
