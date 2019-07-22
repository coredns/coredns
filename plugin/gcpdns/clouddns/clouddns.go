package clouddns

import (
	"context"

	"github.com/coredns/coredns/plugin/gcpdns/api"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type zonesService struct {
	service *dns.Service
}

type zonesServiceCall struct {
	call *dns.ManagedZonesGetCall
}

func (m *zonesService) Get(project string, managedZone string) api.ManagedZonesGetCall {
	return &zonesServiceCall{m.service.ManagedZones.Get(project, managedZone)}
}

func (c *zonesServiceCall) Context(ctx context.Context) api.ManagedZonesGetCall {
	c.call = c.call.Context(ctx)
	return c
}

func (c *zonesServiceCall) Do(opts ...googleapi.CallOption) (*dns.ManagedZone, error) {
	return c.call.Do(opts...)
}

type rrSetsService struct {
	service *dns.Service
}

type rrSetsServiceCall struct {
	call *dns.ResourceRecordSetsListCall
}

func (r *rrSetsService) List(project string, managedZone string) api.ResourceRecordSetsListCall {
	return &rrSetsServiceCall{call: r.service.ResourceRecordSets.List(project, managedZone)}
}

func (c *rrSetsServiceCall) Context(ctx context.Context) api.ResourceRecordSetsListCall {
	c.call = c.call.Context(ctx)
	return c
}

func (c *rrSetsServiceCall) Do(opts ...googleapi.CallOption) (*dns.ResourceRecordSetsListResponse, error) {
	return c.call.Do(opts...)
}

// NewService creates a new Service whose implementation is the GCP DNS API.
func NewService(ctx context.Context, opts ...option.ClientOption) (*api.Service, error) {
	s, err := dns.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &api.Service{
		ManagedZones:       &zonesService{service: s},
		ResourceRecordSets: &rrSetsService{service: s},
	}, nil
}
