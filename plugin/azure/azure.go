package azure

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"

	azure "github.com/Azure/azure-sdk-for-go/profiles/latest/dns/mgmt/dns"
	"github.com/miekg/dns"
)

type zone struct {
	id   string
	z    *file.Zone
	zone string
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
				return nil, err
			}
			// Normalizing zoneName to make it fqdn if required
			zoneNameFQDN := dns.Fqdn(zoneName)
			if _, ok := zones[zoneNameFQDN]; !ok {
				zoneNames = append(zoneNames, zoneNameFQDN)
			}
			zones[zoneNameFQDN] = append(zones[zoneNameFQDN], &zone{id: resourceGroup, zone: zoneName, z: file.NewZone(zoneName, "")})
		}
	}
	return &Azure{
		client:    dnsClient,
		zones:     zones,
		zoneNames: zoneNames,
		upstream:  up,
	}, nil
}

// Run updates the zones once and starts a goroutine to do it every minute.
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
				if err := h.updateZones(ctx); err != nil && ctx.Err() == nil {
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
		go func(zName string, z []*zone) {
			var err error
			defer func() {
				errc <- err
			}()

			for i, hostedZone := range z {
				recordSet, err := h.client.ListByDNSZone(ctx, hostedZone.id, hostedZone.zone, nil, "")
				if err != nil {
					err = fmt.Errorf("failed to list resource records for %v from azure: %v", hostedZone.zone, err)
					return
				}
				newZ := updateZoneFromResourceSet(recordSet, zName)
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
	return nil
}

func updateZoneFromResourceSet(recordSet azure.RecordSetListResultPage, zName string) *file.Zone {
	newZ := file.NewZone(zName, "")
	for _, result := range *(recordSet.Response().Value) {
		if result.RecordSetProperties.ARecords != nil {
			for _, a := range *(result.RecordSetProperties.ARecords) {
				rfc1035 := fmt.Sprintf("%s %d IN %s %s", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), "A", *(a.Ipv4Address))
				r, err := dns.NewRR(rfc1035)
				if err != nil {
					log.Warningf("failed to process resource record set: %v", err.Error())
				}
				newZ.Insert(r)
			}
		}

		if result.RecordSetProperties.AaaaRecords != nil {
			for _, a := range *(result.RecordSetProperties.AaaaRecords) {
				rfc1035 := fmt.Sprintf("%s %d IN %s %s", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), "AAAA", *(a.Ipv6Address))
				r, err := dns.NewRR(rfc1035)
				if err != nil {
					log.Warningf("failed to process resource record set: %v", err.Error())
				}
				newZ.Insert(r)
			}
		}

		if result.RecordSetProperties.MxRecords != nil {
			for _, a := range *(result.RecordSetProperties.MxRecords) {
				// vachdal.my.private. 300 IN MX 10 mailserver.example.com.
				rfc1035 := fmt.Sprintf("%s %d IN MX %d %s", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), *(a.Preference), *(a.Exchange))
				r, err := dns.NewRR(rfc1035)
				if err != nil {
					log.Warningf("failed to process resource record set: %v", err.Error())
				}
				newZ.Insert(r)
			}
		}

		if result.RecordSetProperties.PtrRecords != nil {
			for _, a := range *(result.RecordSetProperties.PtrRecords) {
				// tetoda.my.private. 300 IN PTR www.tetoda.com
				rfc1035 := fmt.Sprintf("%s %d IN PTR %s", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), *(a.Ptrdname))
				r, err := dns.NewRR(rfc1035)
				if err != nil {
					log.Warningf("failed to process resource record set: %v", err.Error())
				}
				newZ.Insert(r)
			}
		}

		if result.RecordSetProperties.SrvRecords != nil {
			for _, a := range *(result.RecordSetProperties.SrvRecords) {
				// kumor.my.private. 300 IN SRV 1 10 5269 xmpp-server.example.com.
				rfc1035 := fmt.Sprintf("%s %d IN SRV %d %d %d %s", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), *(a.Priority), *(a.Weight), *(a.Port), *(a.Target))
				r, err := dns.NewRR(rfc1035)
				if err != nil {
					log.Warningf("failed to process resource record set: %v", err.Error())
				}
				newZ.Insert(r)
			}
		}

		if result.RecordSetProperties.TxtRecords != nil {
			for _, txt := range *(result.RecordSetProperties.TxtRecords) {
				// fagutra.my.private. 300 IN TXT "CoreDNS is awesome"
				rfc1035 := fmt.Sprintf("%s %d IN TXT \"%s\"", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), strings.Join(*(txt.Value), " "))
				r, err := dns.NewRR(rfc1035)
				if err != nil {
					log.Warningf("failed to process resource record set: %v", err.Error())
				}
				newZ.Insert(r)
			}
		}

		if result.RecordSetProperties.NsRecords != nil {
			for _, a := range *(result.RecordSetProperties.NsRecords) {
				rfc1035 := fmt.Sprintf("%s %d IN %s %s", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), "NS", *(a.Nsdname))
				r, err := dns.NewRR(rfc1035)
				if err != nil {
					log.Warningf("failed to process resource record set: %v", err.Error())
				}
				newZ.Insert(r)
			}
		}

		if result.RecordSetProperties.SoaRecord != nil {
			a := result.RecordSetProperties.SoaRecord
			rfc1035 := fmt.Sprintf("%s %d IN %s %s", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), "SOA", fmt.Sprintf("%s %s %d %d %d %d %d", *(a.Host), *(a.Email), *(a.SerialNumber), *(a.RefreshTime), *(a.RetryTime), *(a.ExpireTime), *(a.MinimumTTL)))
			r, err := dns.NewRR(rfc1035)
			if err != nil {
				log.Warningf("failed to process resource record set: %v", err.Error())
			}
			newZ.Insert(r)
		}

		if result.RecordSetProperties.CnameRecord != nil {
			rfc1035 := fmt.Sprintf("%s %d IN CNAME %s", *(result.RecordSetProperties.Fqdn), *(result.RecordSetProperties.TTL), *(result.RecordSetProperties.CnameRecord.Cname))
			r, err := dns.NewRR(rfc1035)
			if err != nil {
				log.Warningf("failed to process resource record set: %v", err.Error())
			}
			newZ.Insert(r)
		}
	}
	return newZ
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
