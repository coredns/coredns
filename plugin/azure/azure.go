package azure

import (
	"context"
	"fmt"
	"sync"
	"time"

	azure "github.com/Azure/azure-sdk-for-go/profiles/latest/dns/mgmt/dns"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

type zone struct {
	id  string
	z   *file.Zone
	dns string
}

type zones map[string][]*zone

// Azure is the core struct of the azure plugin
type Azure struct {
	Next      plugin.Handler
	Fall      fall.F
	zoneNames []string
	client    azure.RecordSetsClient
	upstream  *upstream.Upstream
	zMu       sync.RWMutex
	zones     zones
}

// New validates the input dns zones and initializes the Azure struct
func New(ctx context.Context, dnsClient azure.RecordSetsClient, keys map[string][]string, up *upstream.Upstream) (*Azure, error) {
	zones := make(map[string][]*zone, len(keys))
	zoneNames := make([]string, 0, len(keys))
	for resourceGroup, inputZoneNames := range keys {
		for _, zoneName := range inputZoneNames {
			_, err := dnsClient.ListAllByDNSZone(context.Background(), resourceGroup, zoneName, nil, "")
			if err != nil {
				log.Fatalf("err not nil:%s", err.Error())
			}

			if _, ok := zones[zoneName]; !ok {
				zoneNames = append(zoneNames, fmt.Sprintf("%s.", zoneName))
			}
			zones[fmt.Sprintf("%s.", zoneName)] = append(zones[zoneName], &zone{id: resourceGroup, dns: zoneName, z: file.NewZone(zoneName, "")})
		}
	}
	return &Azure{
		client:    dnsClient,
		zones:     zones,
		zoneNames: zoneNames,
		upstream:  up,
	}, nil
}

// Run updates the zones once and starts a goroutine to do it every minute
func (h *Azure) Run(ctx context.Context) error {
	if err := h.updateZones(ctx); err != nil {
		return err
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Infof("Breaking out of Azure update loop: %v", ctx.Err())
				return
			case <-time.After(1 * time.Minute):
				if err := h.updateZones(ctx); err != nil && ctx.Err() == nil /* Don't log error if ctx expired. */ {
					log.Errorf("Failed to update zones: %v", err)
				}
			}
		}
	}()
	return nil
}

func (h *Azure) updateZones(ctx context.Context) error {
	errc := make(chan error)
	defer close(errc)
	for zName, z := range h.zones {
		// zName - docker-machine
		// z - foo.com, AAA, SOA
		go func(zName string, z []*zone) {
			var err error
			defer func() {
				errc <- err
			}()

			for i, hostedZone := range z {
				recordSet, err := h.client.ListByDNSZone(context.Background(), hostedZone.id, hostedZone.dns, nil, "")
				if err != nil {
					log.Fatalf("in update, err not nil: %s", err.Error())
				}

				newZ, err := updateZoneFromResourceSet(recordSet, hostedZone.dns)
				if err != nil {
					log.Fatalf("in update, err not nil: %s", err.Error())
				}
				newZ.Upstream = h.upstream
				h.zMu.Lock()
				(*z[i]).z = newZ
				h.zMu.Unlock()
			}
		}(zName, z)
	}
	// Collect errors (if any). This will also sync on all zones updates
	// completion.
	var errs []string
	for i := 0; i < len(h.zones); i++ {
		err := <-errc
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) != 0 {
		return fmt.Errorf("errors updating zones: %v", errs)
	}
	return nil //
}

func updateZoneFromResourceSet(recordSet azure.RecordSetListResultPage, hostedZoneDNS string) (*file.Zone, error) {
	newZ := file.NewZone(hostedZoneDNS, "")
	for _, result := range *(recordSet.Response().Value) {
		if result.RecordSetProperties.ARecords != nil {
			for _, a := range *(result.RecordSetProperties.ARecords) {
				rfc1035 := fmt.Sprintf("%s %d IN %s %s", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), "A", *(a.Ipv4Address))
				r, err := dns.NewRR(rfc1035)
				if err != nil {
					log.Fatalf("CANNOT typecast to coredns: %s", err.Error())
				}
				newZ.Insert(r)
			}
		}

		if result.RecordSetProperties.AaaaRecords != nil {
			for _, a := range *(result.RecordSetProperties.AaaaRecords) {
				rfc1035 := fmt.Sprintf("%s %d IN %s %s", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), "AAAA", *(a.Ipv6Address))
				r, err := dns.NewRR(rfc1035)
				if err != nil {
					log.Fatalf("CANNOT typecast to coredns: %s", err.Error())
				}
				newZ.Insert(r)
			}
		}

		if result.RecordSetProperties.NsRecords != nil {
			for _, a := range *(result.RecordSetProperties.NsRecords) {
				rfc1035 := fmt.Sprintf("%s %d IN %s %s", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), "NS", *(a.Nsdname))
				r, err := dns.NewRR(rfc1035)
				if err != nil {
					log.Fatalf("CANNOT typecast to coredns: %s", err.Error())
				}
				newZ.Insert(r)
			}
		}

		if result.RecordSetProperties.SoaRecord != nil {
			a := result.RecordSetProperties.SoaRecord
			rfc1035 := fmt.Sprintf("%s %d IN %s %s", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), "SOA", fmt.Sprintf("%s %s %d %d %d %d %d", *(a.Host), *(a.Email), *(a.SerialNumber), *(a.RefreshTime), *(a.RetryTime), *(a.ExpireTime), *(a.MinimumTTL)))
			r, err := dns.NewRR(rfc1035)
			if err != nil {
				log.Fatalf("CANNOT typecast to coredns: %s", err.Error())
			}
			newZ.Insert(r)
		}
	}
	return newZ, nil
}

// ServeDNS uses the azure plugin to serve dns requests
func (h *Azure) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	qname := state.Name()

	zName := plugin.Zones(h.zoneNames).Matches(qname)
	if zName == "" {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	z, ok := h.zones[zName]
	if !ok || z == nil {
		return dns.RcodeServerFailure, nil
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	var result file.Result
	for _, hostedZone := range z {
		h.zMu.RLock()
		m.Answer, m.Ns, m.Extra, result = hostedZone.z.Lookup(ctx, state, qname)
		h.zMu.RUnlock()

		// record type exists for this name (NODATA).
		if len(m.Answer) != 0 || result == file.NoData {
			break
		}
	}

	if len(m.Answer) == 0 && result != file.NoData && h.Fall.Through(qname) {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	switch result {
	case file.Success:
	case file.NoData:
	case file.NameError:
		m.Rcode = dns.RcodeNameError
	case file.Delegation:
		m.Authoritative = false
	case file.ServerFailure:
		return dns.RcodeServerFailure, nil
	}

	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// Name implements plugin.Handler.Name.
func (h *Azure) Name() string { return "azure" }
