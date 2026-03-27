package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DnsZoneSpec defines the desired state of a DNS zone.
type DnsZoneSpec struct {
	// Zone is the domain name (e.g., "example.com").
	Zone string `json:"zone"`

	// OrgID is the organization that owns this zone (multi-tenant).
	// +optional
	OrgID string `json:"orgId,omitempty"`

	// CloudflareZoneID links this zone to a Cloudflare zone for edge sync.
	// +optional
	CloudflareZoneID string `json:"cloudflareZoneId,omitempty"`

	// SyncToCoreDNS controls whether records are pushed to the CoreDNS hanzodns.
	// +kubebuilder:default=true
	SyncToCoreDNS bool `json:"syncToCoreDNS,omitempty"`

	// CoreDNSEndpoint is the hanzodns sync endpoint URL.
	// +kubebuilder:default="http://coredns-hanzodns.dns-system.svc:8443"
	CoreDNSEndpoint string `json:"coreDNSEndpoint,omitempty"`
}

// DnsZoneStatus defines the observed state of DnsZone.
type DnsZoneStatus struct {
	// Phase represents the current lifecycle phase.
	// +kubebuilder:validation:Enum=Pending;Active;Syncing;Error
	Phase string `json:"phase,omitempty"`

	// RecordCount is the number of DnsRecord resources in this zone.
	RecordCount int `json:"recordCount,omitempty"`

	// LastSyncTime is when records were last pushed to CoreDNS.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// LastCloudflareSync is when records were last synced to Cloudflare.
	// +optional
	LastCloudflareSync *metav1.Time `json:"lastCloudflareSync,omitempty"`

	// Conditions represent the latest available observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Zone",type=string,JSONPath=`.spec.zone`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Records",type=integer,JSONPath=`.status.recordCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DnsZone is the Schema for the dnszones API.
type DnsZone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DnsZoneSpec   `json:"spec,omitempty"`
	Status DnsZoneStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DnsZoneList contains a list of DnsZone.
type DnsZoneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DnsZone `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DnsZone{}, &DnsZoneList{})
}
