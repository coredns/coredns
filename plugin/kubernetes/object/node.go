package object

import (
	api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"net"
)

// Pod is a stripped down api.Pod with only the items we need for CoreDNS.
type Node struct {
	// Don't add new fields to this struct without talking to the CoreDNS maintainers.
	Version     string
	InternalIPS []string
	ExternalIPS []string
	Name        string
	Namespace   string
	Index       string

	*Empty
}

// ToPod returns a function that converts an api.Pod to a *Pod.
func ToNode(skipCleanup bool) ToFunc {
	return func(obj interface{}) interface{} {
		return toNode(skipCleanup, obj)
	}
}

func toNode(skipCleanup bool, obj interface{}) interface{} {
	node, ok := obj.(*api.Node)
	if !ok {
		return nil
	}

	name := node.GetName()
	if net.ParseIP(name) != nil {
		return nil
	}
	var internalIPS []string
	var externalIPS []string
	for _, addr := range node.Status.Addresses {
		nodeIP := addr.Address
		if addr.Type == api.NodeInternalIP {

			internalIPS = append(internalIPS, nodeIP)
		}
		if addr.Type == api.NodeExternalIP {
			externalIPS = append(externalIPS, nodeIP)
		}
	}

	p := &Node{
		Version:     node.GetResourceVersion(),
		InternalIPS: internalIPS,
		ExternalIPS: externalIPS,
		Name:        name,
		Namespace:   "default",
		Index:       name,
	}
	// don't add nodes that are being deleted.
	t := node.ObjectMeta.DeletionTimestamp
	if t != nil && !(*t).Time.IsZero() {
		return nil
	}

	if !skipCleanup {
		*node = api.Node{}
	}

	return p
}

var _ runtime.Object = &Node{}

// DeepCopyObject implements the ObjectKind interface.
func (p *Node) DeepCopyObject() runtime.Object {
	p1 := &Node{
		Version:     p.Version,
		InternalIPS: p.InternalIPS,
		ExternalIPS: p.ExternalIPS,
		Name:        p.Name,
		Namespace:   p.Namespace,
		Index:       p.Index,
	}
	return p1
}

// GetNamespace implements the metav1.Object interface.
func (p *Node) GetNamespace() string { return p.Namespace }

// SetNamespace implements the metav1.Object interface.
func (p *Node) SetNamespace(namespace string) {}

// GetName implements the metav1.Object interface.
func (p *Node) GetName() string { return p.Name }

// SetName implements the metav1.Object interface.
func (p *Node) SetName(name string) {}

// GetResourceVersion implements the metav1.Object interface.
func (p *Node) GetResourceVersion() string { return p.Version }

// SetResourceVersion implements the metav1.Object interface.
func (p *Node) SetResourceVersion(version string) {}
