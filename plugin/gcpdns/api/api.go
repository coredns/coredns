package api

import (
	"context"

	"google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"
)

// Service holds a mockable interface to the GCP Cloud DNS API.  A custom
// interface could have been used instead, but it is felt that the mocking
// setup should closely resemble the actual GCP Cloud DNS API.
type Service struct {
	ManagedZones       ManagedZonesService
	ResourceRecordSets ResourceRecordSetsService
}

// ManagedZonesService is a mockable interface to the GCP Cloud DNS API.
type ManagedZonesService interface {
	// Get: Fetch the representation of an existing ManagedZone.
	Get(project string, managedZone string) ManagedZonesGetCall
}

// ManagedZonesGetCall is a mockable interface to the GCP Cloud DNS API.
type ManagedZonesGetCall interface {
	// Context sets the context to be used in this call's Do method. Any
	// pending HTTP request will be aborted if the provided context is
	// canceled.
	Context(ctx context.Context) ManagedZonesGetCall

	// Do executes the "dns.managedZones.get" call.
	// Exactly one of *ManagedZone or error will be non-nil. Any non-2xx
	// status code is an error. Response headers are in either
	// *ManagedZone.ServerResponse.Header or (if a response was returned at
	// all) in error.(*googleapi.Error).Header. Use googleapi.IsNotModified
	// to check whether the returned error was because
	// http.StatusNotModified was returned.
	Do(opts ...googleapi.CallOption) (*dns.ManagedZone, error)
}

// ResourceRecordSetsService is a mockable interface to the GCP Cloud DNS API.
type ResourceRecordSetsService interface {
	// List: Enumerate ResourceRecordSets that have been created but not yet
	// deleted.
	List(project string, managedZone string) ResourceRecordSetsListCall
}

// ResourceRecordSetsListCall is a mockable interface to the GCP Cloud DNS API.
type ResourceRecordSetsListCall interface {
	// Context sets the context to be used in this call's Do method. Any
	// pending HTTP request will be aborted if the provided context is
	// canceled.
	Context(ctx context.Context) ResourceRecordSetsListCall

	// Do executes the "dns.resourceRecordSets.list" call.
	// Exactly one of *ResourceRecordSetsListResponse or error will be
	// non-nil. Any non-2xx status code is an error. Response headers are in
	// either *ResourceRecordSetsListResponse.ServerResponse.Header or (if a
	// response was returned at all) in error.(*googleapi.Error).Header. Use
	// googleapi.IsNotModified to check whether the returned error was
	// because http.StatusNotModified was returned.
	Do(opts ...googleapi.CallOption) (*dns.ResourceRecordSetsListResponse, error)
}
