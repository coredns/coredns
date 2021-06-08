package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ForwardSpec represents the spec of a Forward
type ForwardSpec struct {
	From string   `json:"from,omitempty"`
	To   []string `json:"to,omitempty"`
}

// ForwardStatus represents the status of a Forward
type ForwardStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="From",type=string,JSONPath=`.spec.from`
// +kubebuilder:printcolumn:name="To",type=string,JSONPath=`.spec.to`

// Forward represents a zone that should have its DNS requests forwarded to an
// upstream DNS server within CoreDNS
type Forward struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ForwardSpec   `json:"spec,omitempty"`
	Status ForwardStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ForwardList represents a list of Forwards
type ForwardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Forward `json:"items"`
}
