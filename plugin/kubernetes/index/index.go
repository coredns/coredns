package index

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Object is an interface that each of our object (services, endpoints, etc.) need to implement.
type Object interface {
	UniqueKey() string
}

// ToFunc converts any type to an Object.
type ToFunc func(interface{}) runtime.Object

// Empty is an empty struct.
type Empty struct{}

// GetObjectKind implements the ObjectKind interface as a noop.
func (e *Empty) GetObjectKind() schema.ObjectKind { return schema.EmptyObjectKind }
