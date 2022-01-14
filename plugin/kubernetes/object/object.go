// Package object holds functions that convert the objects from the k8s API in
// to a more memory efficient structures.
//
// Adding new fields to any of the structures defined in pod.go, endpoint.go
// and service.go should not be done lightly as this increases the memory use
// and will leads to OOMs in the k8s scale test.
//
// We can do some optimizations here as well. We store IP addresses as strings,
// this might be moved to uint32 (for v4) for instance, but then we need to
// convert those again.
//
// Also the msg.Service use in this plugin may be deprecated at some point, as
// we don't use most of those features anyway and would free us from the *etcd*
// dependency, where msg.Service is defined. And should save some mem/cpu as we
// convert to and from msg.Services.
package object

import (
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

// ToFunc converts one v1.Object to another v1.Object.
type ToFunc func(v1.Object) (v1.Object, error)

// ProcessorBuilder returns function to process cache events.
type ProcessorBuilder func(cache.Indexer, cache.ResourceEventHandler) cache.ProcessFunc

// Empty is an empty struct.
type Empty struct{}

// GetObjectKind implements the ObjectKind interface as a noop.
func (e *Empty) GetObjectKind() schema.ObjectKind { return schema.EmptyObjectKind }

// GetGenerateName implements the metav1.Object interface.
func (e *Empty) GetGenerateName() string { return "" }

// SetGenerateName implements the metav1.Object interface.
func (e *Empty) SetGenerateName(name string) {}

// GetUID implements the metav1.Object interface.
func (e *Empty) GetUID() types.UID { return "" }

// SetUID implements the metav1.Object interface.
func (e *Empty) SetUID(uid types.UID) {}

// GetGeneration implements the metav1.Object interface.
func (e *Empty) GetGeneration() int64 { return 0 }

// SetGeneration implements the metav1.Object interface.
func (e *Empty) SetGeneration(generation int64) {}

// GetSelfLink implements the metav1.Object interface.
func (e *Empty) GetSelfLink() string { return "" }

// SetSelfLink implements the metav1.Object interface.
func (e *Empty) SetSelfLink(selfLink string) {}

// GetCreationTimestamp implements the metav1.Object interface.
func (e *Empty) GetCreationTimestamp() v1.Time { return v1.Time{} }

// SetCreationTimestamp implements the metav1.Object interface.
func (e *Empty) SetCreationTimestamp(timestamp v1.Time) {}

// GetDeletionTimestamp implements the metav1.Object interface.
func (e *Empty) GetDeletionTimestamp() *v1.Time { return &v1.Time{} }

// SetDeletionTimestamp implements the metav1.Object interface.
func (e *Empty) SetDeletionTimestamp(timestamp *v1.Time) {}

// GetDeletionGracePeriodSeconds implements the metav1.Object interface.
func (e *Empty) GetDeletionGracePeriodSeconds() *int64 { return nil }

// SetDeletionGracePeriodSeconds implements the metav1.Object interface.
func (e *Empty) SetDeletionGracePeriodSeconds(*int64) {}

// GetLabels implements the metav1.Object interface.
func (e *Empty) GetLabels() map[string]string { return nil }

// SetLabels implements the metav1.Object interface.
func (e *Empty) SetLabels(labels map[string]string) {}

// GetAnnotations implements the metav1.Object interface.
func (e *Empty) GetAnnotations() map[string]string { return nil }

// SetAnnotations implements the metav1.Object interface.
func (e *Empty) SetAnnotations(annotations map[string]string) {}

// GetFinalizers implements the metav1.Object interface.
func (e *Empty) GetFinalizers() []string { return nil }

// SetFinalizers implements the metav1.Object interface.
func (e *Empty) SetFinalizers(finalizers []string) {}

// GetOwnerReferences implements the metav1.Object interface.
func (e *Empty) GetOwnerReferences() []v1.OwnerReference { return nil }

// SetOwnerReferences implements the metav1.Object interface.
func (e *Empty) SetOwnerReferences([]v1.OwnerReference) {}

// GetClusterName implements the metav1.Object interface.
func (e *Empty) GetClusterName() string { return "" }

// SetClusterName implements the metav1.Object interface.
func (e *Empty) SetClusterName(clusterName string) {}

// GetManagedFields implements the metav1.Object interface.
func (e *Empty) GetManagedFields() []v1.ManagedFieldsEntry { return nil }

// SetManagedFields implements the metav1.Object interface.
func (e *Empty) SetManagedFields(managedFields []v1.ManagedFieldsEntry) {}

// stripLeadingZerosIPv4 strips leading zeros from IPv4 addresses.  Kubernetes (1.23) permits IPv4s with leading zeros,
// and interprets them as decimal.  So, when storing an IP address from a Kubernetes API Object, we should strip
// leading zeros so that later net.ParseIP/net.ParseCIDR operations will not fail.
//
// From the Kubertnetes 1.23 release notes:
// Since golang 1.17 both net.ParseIP and net.ParseCIDR rejects leading zeros in the dot-decimal notation of IPv4
// addresses, Kubernetes will keep allowing leading zeros on IPv4 address to not break the compatibility.
// IMPORTANT: Kubernetes interprets leading zeros on IPv4 addresses as decimal, users must not rely on parser alignment
// to not being impacted by the associated security advisory: CVE-2021-29923 golang standard library "net" - Improper
// Input Validation of octal literals in golang 1.16.2 and below standard library "net" results in indeterminate SSRF
// & RFI vulnerabilities.
//
func stripLeadingZerosIPv4(ip string) string {
	if strings.Contains(ip, ":") {
		return ip
	}
	octets := strings.Split(ip, ".")
	if len(octets) != 4 {
		return ip
	}
	for i := range octets {
		if len(octets[i]) < 2 {
			continue
		}
		j := 0
		for j < len(octets[i])-1 {
			if octets[i][j] != '0' {
				break
			}
			j++
		}
		octets[i] = octets[i][j:]
	}
	return strings.Join(octets, ".")
}

func copyAndStripLeadingZerosIPv4(dst, src []string) int {
	for i := range src {
		dst[i] = stripLeadingZerosIPv4(src[i])
	}
	return len(dst)
}
