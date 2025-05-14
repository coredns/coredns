package object

import (
	"maps"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	mcs "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

// Endpoints is a stripped down api.Endpoints with only the items we need for CoreDNS.
type MultiClusterEndpoints struct {
	Endpoints
	ClusterId string
	*Empty
}

// MultiClusterEndpointsKey returns a string using for the index.
func MultiClusterEndpointsKey(name, namespace string) string { return name + "." + namespace }

// EndpointSliceToEndpoints converts a *discovery.EndpointSlice to a *Endpoints.
func EndpointSliceToMultiClusterEndpoints(obj meta.Object) (meta.Object, error) {
	labels := maps.Clone(obj.GetLabels())
	ends, err := EndpointSliceToEndpoints(obj)
	if err != nil {
		return nil, err
	}
	e := &MultiClusterEndpoints{
		Endpoints: *ends.(*Endpoints),
		ClusterId: labels[mcs.LabelSourceCluster],
	}
	e.Index = MultiClusterEndpointsKey(labels[mcs.LabelServiceName], ends.GetNamespace())

	return e, nil
}

var _ runtime.Object = &Endpoints{}

// DeepCopyObject implements the ObjectKind interface.
func (e *MultiClusterEndpoints) DeepCopyObject() runtime.Object {
	e1 := &MultiClusterEndpoints{
		ClusterId: e.ClusterId,
		Endpoints: *e.Endpoints.DeepCopyObject().(*Endpoints),
	}
	return e1
}

// GetNamespace implements the metav1.Object interface.
func (e *MultiClusterEndpoints) GetNamespace() string { return e.Endpoints.GetNamespace() }

// SetNamespace implements the metav1.Object interface.
func (e *MultiClusterEndpoints) SetNamespace(namespace string) {}

// GetName implements the metav1.Object interface.
func (e *MultiClusterEndpoints) GetName() string { return e.Endpoints.GetName() }

// SetName implements the metav1.Object interface.
func (e *MultiClusterEndpoints) SetName(name string) {}

// GetResourceVersion implements the metav1.Object interface.
func (e *MultiClusterEndpoints) GetResourceVersion() string { return e.Endpoints.GetResourceVersion() }

// SetResourceVersion implements the metav1.Object interface.
func (e *MultiClusterEndpoints) SetResourceVersion(version string) {}
