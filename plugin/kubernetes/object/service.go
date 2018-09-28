package object

import (
	api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Service is a stripped down api.Service with only the items we need for CoreDNS.
type Service struct {
	Version      string
	Name         string
	Namespace    string
	Index        string
	ClusterIP    string
	Type         api.ServiceType
	ExternalName string
	Ports        []api.ServicePort

	*Empty
}

// ServiceKey return a string using for the index.
func ServiceKey(name, namespace string) string { return name + "." + namespace }

// ToService converts an api.Service to a *Service.
func ToService(obj interface{}) interface{} {
	svc, ok := obj.(*api.Service)
	if !ok {
		return nil
	}

	s := &Service{
		Version:      svc.GetResourceVersion(),
		Name:         svc.GetName(),
		Namespace:    svc.GetNamespace(),
		Index:        ServiceKey(svc.GetName(), svc.GetNamespace()),
		ClusterIP:    svc.Spec.ClusterIP,
		Type:         svc.Spec.Type,
		ExternalName: svc.Spec.ExternalName,
	}

	ports := len(svc.Spec.Ports)
	if ports == 0 {
		s.Ports = []api.ServicePort{{Port: -1}}
	} else {
		s.Ports = make([]api.ServicePort, ports)
		copy(s.Ports, svc.Spec.Ports)
	}

	return s
}

var _ runtime.Object = &Service{}

// GetNamespace implements the metav1.Object interface.
func (s *Service) GetNamespace() string { return s.Namespace }

// SetNamespace implements the metav1.Object interface.
func (s *Service) SetNamespace(namespace string) {}

// GetName implements the metav1.Object interface.
func (s *Service) GetName() string { return s.Name }

// SetName implements the metav1.Object interface.
func (s *Service) SetName(name string) {}

// GetResourceVersion implements the metav1.Object interface.
func (s *Service) GetResourceVersion() string { return s.Version }

// SetResourceVersion implements the metav1.Object interface.
func (s *Service) SetResourceVersion(version string) {}
