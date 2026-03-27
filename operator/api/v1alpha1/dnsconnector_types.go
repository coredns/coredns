package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DnsConnectorSpec configures a connection to the CoreDNS hanzodns instance.
type DnsConnectorSpec struct {
	// Endpoint is the hanzodns HTTP API address.
	// +kubebuilder:default="http://coredns-hanzodns.dns-system.svc:8443"
	Endpoint string `json:"endpoint"`

	// APIKeySecretRef references a Secret containing the API key.
	// +optional
	APIKeySecretRef *SecretKeyRef `json:"apiKeySecretRef,omitempty"`

	// SyncIntervalSeconds is how often the operator syncs state.
	// +kubebuilder:default=30
	SyncIntervalSeconds int `json:"syncIntervalSeconds,omitempty"`

	// PostgresConnectionRef references a Secret with DATABASE_URL.
	// +optional
	PostgresConnectionRef *SecretKeyRef `json:"postgresConnectionRef,omitempty"`
}

// SecretKeyRef references a key within a Kubernetes Secret.
type SecretKeyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// DnsConnectorStatus defines the observed state of DnsConnector.
type DnsConnectorStatus struct {
	// Connected indicates whether the operator can reach the CoreDNS API.
	Connected bool `json:"connected,omitempty"`

	// LastCheckTime is when connectivity was last verified.
	// +optional
	LastCheckTime *metav1.Time `json:"lastCheckTime,omitempty"`

	// Conditions represent the latest available observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.spec.endpoint`
// +kubebuilder:printcolumn:name="Connected",type=boolean,JSONPath=`.status.connected`

// DnsConnector configures the operator's connection to CoreDNS.
type DnsConnector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DnsConnectorSpec   `json:"spec,omitempty"`
	Status DnsConnectorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DnsConnectorList contains a list of DnsConnector.
type DnsConnectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DnsConnector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DnsConnector{}, &DnsConnectorList{})
}
