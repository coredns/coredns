package object

import (
	"fmt"

	api "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Namespace is a stripped down api.Namespace with only the items we need for CoreDNS.
type Namespace struct {
	// Don't add new fields to this struct without talking to the CoreDNS maintainers.
	Version string
	Name    string

	*Empty
}

// ToNamespace returns a function that converts an api.Namespace to a *Namespace.
func ToNamespace(obj meta.Object) (meta.Object, error) {
	ns, ok := obj.(*api.Namespace)
	if !ok {
		return nil, fmt.Errorf("unexpected object %v", obj)
	}
	n := &Namespace{
		Version: ns.GetResourceVersion(),
		Name:    ns.GetName(),
	}
	*ns = api.Namespace{}
	return n, nil
}

var _ runtime.Object = &Namespace{}

// DeepCopyObject implements the ObjectKind interface.
func (n *Namespace) DeepCopyObject() runtime.Object {
	n1 := &Namespace{
		Version: n.Version,
		Name:    n.Name,
	}
	return n1
}

// GetNamespace implements the metav1.Object interface.
func (n *Namespace) GetNamespace() string { return "" }

// SetNamespace implements the metav1.Object interface.
func (n *Namespace) SetNamespace(namespace string) {}

// GetName implements the metav1.Object interface.
func (n *Namespace) GetName() string { return n.Name }

// SetName implements the metav1.Object interface.
func (n *Namespace) SetName(name string) {}

// GetResourceVersion implements the metav1.Object interface.
func (n *Namespace) GetResourceVersion() string { return n.Version }

// SetResourceVersion implements the metav1.Object interface.
func (n *Namespace) SetResourceVersion(version string) {}
