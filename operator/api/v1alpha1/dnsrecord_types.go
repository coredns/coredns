package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DnsRecordSpec defines the desired state of a DNS record.
type DnsRecordSpec struct {
	// ZoneRef is the name of the DnsZone this record belongs to.
	ZoneRef string `json:"zoneRef"`

	// Name is the record name (e.g., "www", "@").
	Name string `json:"name"`

	// Type is the DNS record type.
	// +kubebuilder:validation:Enum=A;AAAA;CNAME;MX;TXT;SRV;NS;CAA
	Type string `json:"type"`

	// Content is the record value (IP, hostname, text, etc.).
	Content string `json:"content"`

	// TTL is the time-to-live in seconds.
	// +kubebuilder:default=300
	TTL int `json:"ttl,omitempty"`

	// Priority for MX/SRV records.
	// +optional
	Priority *int `json:"priority,omitempty"`

	// Proxied indicates whether the record should be proxied via Cloudflare.
	// +kubebuilder:default=false
	Proxied bool `json:"proxied,omitempty"`

	// SyncToCloudflare controls whether this record is synced to Cloudflare.
	// +kubebuilder:default=false
	SyncToCloudflare bool `json:"syncToCloudflare,omitempty"`
}

// DnsRecordStatus defines the observed state of DnsRecord.
type DnsRecordStatus struct {
	// Phase represents the current lifecycle phase.
	// +kubebuilder:validation:Enum=Pending;Active;Syncing;Error
	Phase string `json:"phase,omitempty"`

	// CloudflareRecordID is the CF record ID if synced.
	// +optional
	CloudflareRecordID string `json:"cloudflareRecordId,omitempty"`

	// LastSyncTime is when the record was last synced.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Zone",type=string,JSONPath=`.spec.zoneRef`
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Content",type=string,JSONPath=`.spec.content`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// DnsRecord is the Schema for the dnsrecords API.
type DnsRecord struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DnsRecordSpec   `json:"spec,omitempty"`
	Status DnsRecordStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DnsRecordList contains a list of DnsRecord.
type DnsRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DnsRecord `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DnsRecord{}, &DnsRecordList{})
}
