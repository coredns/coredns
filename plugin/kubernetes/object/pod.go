package object

import (
	"errors"
	"fmt"

	api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Pod is a stripped down api.Pod with only the items we need for CoreDNS.
type Pod struct {
	// Don't add new fields to this struct without talking to the CoreDNS maintainers.
	Version   string
	PodIP     string
	Name      string
	Namespace string

	*Empty
}

var errPodTerminating = errors.New("pod terminating")

// ToPod returns a function that converts an api.Pod to a *Pod.
func ToPod(skipCleanup bool) ToFunc {
	return func(obj interface{}) (interface{}, error) {
		apiPod, ok := obj.(*api.Pod)
		if !ok {
			return nil, fmt.Errorf("unexpected object %v", obj)
		}
		pod := toPod(skipCleanup, apiPod)
		t := apiPod.ObjectMeta.DeletionTimestamp
		if t != nil && !(*t).Time.IsZero() {
			// if the pod is in the process of termination, return an error so it can be ignored
			// during add/update event processing
			return pod, errPodTerminating
		}
		return pod, nil
	}
}

func toPod(skipCleanup bool, pod *api.Pod) *Pod {
	p := &Pod{
		Version:   pod.GetResourceVersion(),
		PodIP:     pod.Status.PodIP,
		Namespace: pod.GetNamespace(),
		Name:      pod.GetName(),
	}

	if !skipCleanup {
		*pod = api.Pod{}
	}

	return p
}

var _ runtime.Object = &Pod{}

// DeepCopyObject implements the ObjectKind interface.
func (p *Pod) DeepCopyObject() runtime.Object {
	p1 := &Pod{
		Version:   p.Version,
		PodIP:     p.PodIP,
		Namespace: p.Namespace,
		Name:      p.Name,
	}
	return p1
}

// GetNamespace implements the metav1.Object interface.
func (p *Pod) GetNamespace() string { return p.Namespace }

// SetNamespace implements the metav1.Object interface.
func (p *Pod) SetNamespace(namespace string) {}

// GetName implements the metav1.Object interface.
func (p *Pod) GetName() string { return p.Name }

// SetName implements the metav1.Object interface.
func (p *Pod) SetName(name string) {}

// GetResourceVersion implements the metav1.Object interface.
func (p *Pod) GetResourceVersion() string { return p.Version }

// SetResourceVersion implements the metav1.Object interface.
func (p *Pod) SetResourceVersion(version string) {}
