package clouddns

import (
	"context"

	gcp "google.golang.org/api/dns/v1"
)

type gcpDNS interface {
	zoneExists(projectName, hostedZoneName string) error
	listRRSets(ctx context.Context, projectName, hostedZoneName string) (*gcp.ResourceRecordSetsListResponse, error)
}

type gcpClient struct {
	*gcp.Service
}

// zoneExists is a wrapper method around `gcp.Service.ManagedZones.Get`
// it checks if the provided zone name for a given project exists.
func (c gcpClient) zoneExists(projectName, hostedZoneName string) error {
	_, err := c.ManagedZones.Get(projectName, hostedZoneName).Do()
	if err != nil {
		return err
	}
	return nil
}

// listRRSets is a wrapper method around `gcp.Service.ResourceRecordSets.List`
// it fetches and returns the record sets for a hosted zone.
func (c gcpClient) listRRSets(ctx context.Context, projectName, hostedZoneName string) (*gcp.ResourceRecordSetsListResponse, error) {
	req := c.ResourceRecordSets.List(projectName, hostedZoneName)
	var rs []*gcp.ResourceRecordSet
	if err := req.Pages(ctx, func(page *gcp.ResourceRecordSetsListResponse) error {
		for _, rr := range page.Rrsets {
			rs = append(rs, rr)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &gcp.ResourceRecordSetsListResponse{Rrsets: rs}, nil
}
