package object

import (
	"fmt"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	mcs "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

// ServiceImport is a stripped down api.ServiceImport with only the items we need for CoreDNS.
type ServiceImport struct {
	Version    string
	Name       string
	Namespace  string
	Index      string
	ClusterIPs []string
	Type       mcs.ServiceImportType
	Ports      []mcs.ServicePort

	*Empty
}

// ServiceImportKey returns a string using for the index.
func ServiceImportKey(name, namespace string) string { return name + "." + namespace }

// ToServiceImport converts an v1alpha1.ServiceImport to a *ServiceImport.
func ToServiceImport(obj meta.Object) (meta.Object, error) {
	svc, ok := obj.(*mcs.ServiceImport)

	if !ok {
		return nil, fmt.Errorf("unexpected object %v", obj)
	}
	s := &ServiceImport{
		Version:   svc.GetResourceVersion(),
		Name:      svc.GetName(),
		Namespace: svc.GetNamespace(),
		Index:     ServiceImportKey(svc.GetName(), svc.GetNamespace()),
		Type:      svc.Spec.Type,
	}

	if len(svc.Spec.IPs) > 0 {
		s.ClusterIPs = make([]string, len(svc.Spec.IPs))
		copy(s.ClusterIPs, svc.Spec.IPs)
	}

	if len(svc.Spec.Ports) > 0 {
		s.Ports = make([]mcs.ServicePort, len(svc.Spec.Ports))
		copy(s.Ports, svc.Spec.Ports)
	}

	*svc = mcs.ServiceImport{}
	return s, nil
}

var _ runtime.Object = &ServiceImport{}

// Headless returns true if the service is headless
func (s *ServiceImport) Headless() bool {
	return s.Type == mcs.Headless
}

// DeepCopyObject implements the ObjectKind interface.
func (s *ServiceImport) DeepCopyObject() runtime.Object {
	s1 := &ServiceImport{
		Version:    s.Version,
		Name:       s.Name,
		Namespace:  s.Namespace,
		Index:      s.Index,
		Type:       s.Type,
		ClusterIPs: make([]string, len(s.ClusterIPs)),
		Ports:      make([]mcs.ServicePort, len(s.Ports)),
	}
	copy(s1.ClusterIPs, s.ClusterIPs)
	copy(s1.Ports, s.Ports)
	return s1
}

// GetNamespace implements the metav1.Object interface.
func (s *ServiceImport) GetNamespace() string { return s.Namespace }

// SetNamespace implements the metav1.Object interface.
func (s *ServiceImport) SetNamespace(namespace string) {}

// GetName implements the metav1.Object interface.
func (s *ServiceImport) GetName() string { return s.Name }

// SetName implements the metav1.Object interface.
func (s *ServiceImport) SetName(name string) {}

// GetResourceVersion implements the metav1.Object interface.
func (s *ServiceImport) GetResourceVersion() string { return s.Version }

// SetResourceVersion implements the metav1.Object interface.
func (s *ServiceImport) SetResourceVersion(version string) {}
