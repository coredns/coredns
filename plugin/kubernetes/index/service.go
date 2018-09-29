package index

import (
	api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Service is a stripped down api.Service with only the items we need for CoreDNS.
type Service struct {
	Name         string
	Namespace    string
	ClusterIP    string
	Type         api.ServiceType
	ExternalName string
	Ports        []api.ServicePort
	uniqueKey    string

	*Empty
}

// ServiceKey return a unique key used for indexing this service.
func ServiceKey(name, namespace string) string { return name + "." + namespace }

// ToService converts an api.Service to a *Service.
func ToService(obj interface{}) runtime.Object {
	svc, ok := obj.(*api.Service)
	if !ok {
		return nil
	}

	s := &Service{
		Name:         svc.GetName(),
		Namespace:    svc.GetNamespace(),
		ClusterIP:    svc.Spec.ClusterIP,
		Type:         svc.Spec.Type,
		ExternalName: svc.Spec.ExternalName,
		uniqueKey:    ServiceKey(svc.GetName(), svc.GetNamespace()),
	}
	copy(s.Ports, svc.Spec.Ports)

	return s
}

var _ runtime.Object = &Service{}

// UniqueKey implements the Object interface.
func (s *Service) UniqueKey() string { return s.uniqueKey }

// DeepCopyObject implements the runtime.Object interface.
func (s *Service) DeepCopyObject() runtime.Object {
	s1 := &Service{
		Name:         s.Name,
		Namespace:    s.Namespace,
		ClusterIP:    s.ClusterIP,
		Type:         s.Type,
		ExternalName: s.ExternalName,
		uniqueKey:    s.uniqueKey,
	}
	copy(s1.Ports, s.Ports)
	return s1
}
