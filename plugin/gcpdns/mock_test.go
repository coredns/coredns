package gcpdns

import (
	"context"
	"fmt"

	"github.com/coredns/coredns/plugin/gcpdns/api"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"
)

type rssData struct {
	rType, name, value, hostedZoneID string
}

// org-zone org-zone
// another-org-zone another-org-zone
var data = []rssData{
	{"A", "example.org.", "1.2.3.4", "org-zone"},
	{"A", "www.example.org", "1.2.3.4", "org-zone"},
	{"CNAME", "*.www.example.org", "www.example.org", "org-zone"},
	{"AAAA", "example.org.", "2001:db8:85a3::8a2e:370:7334", "org-zone"},
	{"CNAME", "sample.example.org.", "example.org", "org-zone"},
	{"PTR", "example.org.", "ptr.example.org.", "org-zone"},
	{"SOA", "org.", "ns-cloud-a1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300", "org-zone"},
	{"NS", "com.", "ns-cloud-e1.googledomains.com.", "org-zone"},
	{"A", "split-example.gov.", "1.2.3.4", "org-zone"},
	// Unsupported type should be ignored.
	{"YOLO", "swag.", "foobar", "org-zone"},
	// Hosted zone with the same name, but a different name.
	{"A", "other-example.org.", "3.5.7.9", "another-org-zone"},
	{"A", "split-example.org.", "1.2.3.4", "another-org-zone"},
	{"SOA", "org.", "ns-cloud-d1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300", "another-org-zone"},
	// Hosted zone without SOA.
}

type mockZonesService struct {
	data []rssData
}

type mockZonesServiceCall struct {
	zone *dns.ManagedZone
	err  error
}

func (m *mockZonesService) Get(project string, managedZone string) api.ManagedZonesGetCall {
	if project != "my-project" {
		return &mockZonesServiceCall{
			err: &googleapi.Error{
				Code:    403,
				Message: fmt.Sprintf("Permission denied on resource project %s.", project),
			},
		}
	}

	for _, rss := range m.data {
		if rss.hostedZoneID == managedZone {
			return &mockZonesServiceCall{
				zone: &dns.ManagedZone{
					Name:    rss.hostedZoneID,
					Kind:    rss.rType,
					DnsName: rss.value,
				},
			}
		}
	}

	return &mockZonesServiceCall{
		err: &googleapi.Error{
			Code:    404,
			Message: fmt.Sprintf("The 'parameters.managedZone' resource named '%s' does not exist.", managedZone),
		},
	}
}

func (c *mockZonesServiceCall) Context(ctx context.Context) api.ManagedZonesGetCall {
	return c
}

func (c *mockZonesServiceCall) Do(opts ...googleapi.CallOption) (*dns.ManagedZone, error) {
	return c.zone, c.err
}

type mockRrSetsService struct {
	data []rssData
}

type mockRrSetsServiceCall struct {
	rrsResponse map[string][]*dns.ResourceRecordSet
	project     string
	zone        string
}

func (g *mockRrSetsService) List(project string, managedZone string) api.ResourceRecordSetsListCall {
	rrsResponse := map[string][]*dns.ResourceRecordSet{}
	for _, r := range g.data {
		rrs, ok := rrsResponse[r.hostedZoneID]
		if !ok {
			rrs = make([]*dns.ResourceRecordSet, 0)
		}
		rrs = append(rrs, &dns.ResourceRecordSet{
			Type: r.rType,
			Name: r.name,
			Rrdatas: []string{
				r.value,
			},
			Ttl: 300,
		})
		rrsResponse[r.hostedZoneID] = rrs
	}
	return &mockRrSetsServiceCall{rrsResponse: rrsResponse, zone: managedZone}
}

func (c *mockRrSetsServiceCall) Context(ctx context.Context) api.ResourceRecordSetsListCall {
	return c
}

func (c *mockRrSetsServiceCall) Do(opts ...googleapi.CallOption) (*dns.ResourceRecordSetsListResponse, error) {
	return &dns.ResourceRecordSetsListResponse{Rrsets: c.rrsResponse[c.zone]}, nil
}

func newMock() *api.Service {
	return &api.Service{
		ManagedZones:       &mockZonesService{data: data},
		ResourceRecordSets: &mockRrSetsService{data: data},
	}
}
