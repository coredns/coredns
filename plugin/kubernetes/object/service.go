package object

import (
	"fmt"

	api "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Service is a stripped down api.Service with only the items we need for CoreDNS.
type Service struct {
	// Don't add new fields to this struct without talking to the CoreDNS maintainers.
	Version      string
	Name         string
	Namespace    string
	Index        string
	ClusterIPs   []string
	Type         api.ServiceType
	ExternalName string
	Ports        []api.ServicePort

	// ExternalIPs we may want to export.
	ExternalIPs []string

	*Empty
}

// ServiceKey returns a string using for the index.
func ServiceKey(name, namespace string) string { return name + "." + namespace }

// ToService converts an api.Service to a *Service.
func ToService(obj meta.Object) (meta.Object, error) {
	svc, ok := obj.(*api.Service)
	if !ok {
		return nil, fmt.Errorf("unexpected object %v", obj)
	}
	s := &Service{
		Version:      svc.GetResourceVersion(),
		Name:         svc.GetName(),
		Namespace:    svc.GetNamespace(),
		Index:        ServiceKey(svc.GetName(), svc.GetNamespace()),
		Type:         svc.Spec.Type,
		ExternalName: svc.Spec.ExternalName,

		ExternalIPs: make([]string, len(svc.Status.LoadBalancer.Ingress)+len(svc.Spec.ExternalIPs)),
	}

	if len(svc.Spec.ClusterIPs) > 0 {
		s.ClusterIPs = make([]string, len(svc.Spec.ClusterIPs))
		copy(s.ClusterIPs, svc.Spec.ClusterIPs)
	} else {
		s.ClusterIPs = []string{svc.Spec.ClusterIP}
	}

	if len(svc.Spec.Ports) == 0 {
		// Add sentinel if there are no ports.
		s.Ports = []api.ServicePort{{Port: -1}}
	} else {
		s.Ports = make([]api.ServicePort, len(svc.Spec.Ports))
		copy(s.Ports, svc.Spec.Ports)
	}

	li := copy(s.ExternalIPs, svc.Spec.ExternalIPs)
	for i, lb := range svc.Status.LoadBalancer.Ingress {
		if lb.IP != "" {
			s.ExternalIPs[li+i] = lb.IP
			continue
		}
		s.ExternalIPs[li+i] = lb.Hostname

	}

	*svc = api.Service{}

	return s, nil
}

// Headless returns true if the service is headless
func (s *Service) Headless() bool {
	return s.ClusterIPs[0] == api.ClusterIPNone
}

var _ runtime.Object = &Service{}

// DeepCopyObject implements the ObjectKind interface.
func (s *Service) DeepCopyObject() runtime.Object {
	s1 := &Service{
		Version:      s.Version,
		Name:         s.Name,
		Namespace:    s.Namespace,
		Index:        s.Index,
		Type:         s.Type,
		ExternalName: s.ExternalName,
		ClusterIPs:   make([]string, len(s.ClusterIPs)),
		Ports:        make([]api.ServicePort, len(s.Ports)),
		ExternalIPs:  make([]string, len(s.ExternalIPs)),
	}
	copy(s1.ClusterIPs, s.ClusterIPs)
	copy(s1.Ports, s.Ports)
	copy(s1.ExternalIPs, s.ExternalIPs)
	return s1
}

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
