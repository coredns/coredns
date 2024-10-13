package azure

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	publicAzureDNS "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	publicdns "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	privateAzureDNS "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	privatedns "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	"github.com/miekg/dns"
)

type zone struct {
	// SubscriptionID is the Azure subscription ID
	subscriptionID string
	// resourceGroup
	resourceGroup string
	// TenantID is the Entra ID (formerly AAD) Tenant ID
	tenantID string
	// ClientID is the application/client id of the service principal or managed  identity
	clientID string
	// ClientSecret is the secret assocaited with the service principal (not required if using managed identity)
	clientSecret string
	// The cloud environment
	environment string
	cloudConfig cloud.Configuration
	z           *file.Zone
	// Zone the name of the zone
	zone string
	// private indicates if this is a private or public DNS zone
	private       bool
	publicClient  *publicAzureDNS.RecordSetsClient
	privateClient *privateAzureDNS.RecordSetsClient
}

type zones map[string][]*zone

// Azure is the core struct of the azure plugin.
type Azure struct {
	zoneNames []string
	upstream  *upstream.Upstream
	zMu       sync.RWMutex
	zones     zones

	Next plugin.Handler
	Fall fall.F
}

// Map of string representations to cloud constants
// The keys are aligned to the values used in the now deprecated go-autorest package and
// also with what is defined here https://github.com/Azure/azure-sdk-for-go/blob/main/sdk/azcore/cloud/cloud.go
var cloudTypeMap = map[string]cloud.Configuration{
	"AZUREPUBLIC":            cloud.AzurePublic,
	"AZURECLOUD":             cloud.AzurePublic,
	"AZUREPUBLICCLOUD":       cloud.AzurePublic,
	"AZURECHINA":             cloud.AzureChina,
	"AZURECHINACLOUD":        cloud.AzureChina,
	"AZUREGOVERNMENT":        cloud.AzureGovernment,
	"AZUREUSGOVERNMENT":      cloud.AzureGovernment,
	"AZUREUSGOVERNMENTCLOUD": cloud.AzureGovernment,
}

// isValidCloudType checks if the provided cloud type string is valid.
func isValidCloudType(cloudType string) bool {
	if _, exists := cloudTypeMap[cloudType]; exists {
		return true
	}
	return false
}

// New validates the input DNS zones and initializes the Azure struct.
func New(ctx context.Context, zoneList zones) (*Azure, error) {
	names := make([]string, len(zoneList))

	for zoneName, z := range zoneList {
		for _, zone := range z {
			fqdn := dns.Fqdn(zoneName)
			names = append(names, fqdn)
			// default to AzurePublic if environment is not specified
			cloudType := cloud.AzurePublic
			if zone.environment != "" {
				cloudType = cloudTypeMap[zone.environment]
			}
			zone.cloudConfig = cloudType

			var pubClientFactory *publicAzureDNS.ClientFactory
			var privClientFactory *privateAzureDNS.ClientFactory

			armOptions := arm.ClientOptions{
				ClientOptions: azcore.ClientOptions{
					Cloud: cloudType,
				},
			}

			if zone.clientSecret != "" {
				cred, err := azidentity.NewClientSecretCredential(zone.tenantID, zone.clientID, zone.clientSecret, nil)
				if err != nil {
					return nil, plugin.Error("azure", err)
				}
				pubClientFactory, err = publicAzureDNS.NewClientFactory(zone.subscriptionID, cred, &armOptions)
				if err != nil {
					return nil, plugin.Error("azure", err)
				}
				privClientFactory, err = privateAzureDNS.NewClientFactory(zone.subscriptionID, cred, &armOptions)
				if err != nil {
					return nil, plugin.Error("azure", err)
				}
			} else {
				// We could add a new field in the config for this plugin to specify the managed identity type
				// but we won't do that for backwards compatibility reasons
				clientID := azidentity.ClientID(zone.clientID)
				opts := azidentity.ManagedIdentityCredentialOptions{ID: clientID}
				// Try a user assigned managed identity
				cred, err := azidentity.NewManagedIdentityCredential(&opts)
				if err != nil {
					// If that fails then try a system assigned managed identity
					cred, err = azidentity.NewManagedIdentityCredential(nil)
					if err != nil {
						return nil, plugin.Error("azure", err)
					}
				}
				pubClientFactory, err = publicAzureDNS.NewClientFactory(zone.subscriptionID, cred, &armOptions)
				if err != nil {
					return nil, plugin.Error("azure", err)
				}
				privClientFactory, err = privateAzureDNS.NewClientFactory(zone.subscriptionID, cred, &armOptions)
				if err != nil {
					return nil, plugin.Error("azure", err)
				}
			}

			publicDNSClient := pubClientFactory.NewRecordSetsClient()
			privateDNSClient := privClientFactory.NewRecordSetsClient()

			zone.privateClient = privateDNSClient
			zone.publicClient = publicDNSClient
		}
	}

	return &Azure{
		zones:     zoneList,
		zoneNames: names,
		upstream:  upstream.New(),
	}, nil
}

// Run updates the zone from azure.
func (h *Azure) Run(ctx context.Context) error {
	if err := h.updateZones(ctx); err != nil {
		return err
	}
	go func() {
		delay := 1 * time.Minute
		timer := time.NewTimer(delay)
		defer timer.Stop()
		for {
			timer.Reset(delay)
			select {
			case <-ctx.Done():
				log.Debugf("Breaking out of Azure update loop for %v: %v", h.zoneNames, ctx.Err())
				return
			case <-timer.C:
				if err := h.updateZones(ctx); err != nil && ctx.Err() == nil {
					log.Errorf("Failed to update zones %v: %v", h.zoneNames, err)
				}
			}
		}
	}()
	return nil
}

func (h *Azure) updateZones(ctx context.Context) error {
	var err error
	errs := make([]string, 0)
	for _, zones := range h.zones {
		for _, zoneData := range zones {
			newZ := file.NewZone(zoneData.zone, "")
			zoneName := strings.TrimSuffix(zoneData.zone, ".")
			if zoneData.private {
				pager := zoneData.privateClient.NewListPager(zoneData.resourceGroup, zoneName, nil)
				err = updateZoneFromPrivateResourceSet(pager, newZ)
				if err != nil {
					return err
				}
			} else {
				pager := zoneData.publicClient.NewListByDNSZonePager(zoneData.resourceGroup, zoneName, nil)
				err = updateZoneFromPublicResourceSet(pager, newZ)
				if err != nil {
					return err
				}
			}
			if err != nil {
				errs = append(errs, fmt.Sprintf("failed to list resource records for %v from azure: %v", zoneData.zone, err))
			}
			newZ.Upstream = h.upstream
			h.zMu.Lock()
			zoneData.z = newZ
			h.zMu.Unlock()
		}
	}

	if len(errs) != 0 {
		return fmt.Errorf("errors updating zones: %v", errs)
	}
	return nil
}

func updateZoneFromPublicResourceSet(recordSet *runtime.Pager[publicdns.RecordSetsClientListByDNSZoneResponse], newZ *file.Zone) error {
	ctx := context.Background()
	for recordSet.More() {
		page, err := recordSet.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, v := range page.Value {
			resultFqdn := *v.Properties.Fqdn
			// TODO(vijayt): Azure TTL is int64 but below it expects uint32
			// The maximum value for the TTL can be 2,147,483,647 and
			// the maximum that a uint32 can hold is 4,294,967,295 so this should be ok but check with the maintainers
			resultTTL := uint32(*v.Properties.TTL)
			if v.Properties.ARecords != nil {
				for _, A := range v.Properties.ARecords {
					a := &dns.A{
						Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: resultTTL},
						A:   net.ParseIP(*(A.IPv4Address)),
					}
					newZ.Insert(a)
				}
			}

			if v.Properties.AaaaRecords != nil {
				for _, AAAA := range v.Properties.AaaaRecords {
					aaaa := &dns.AAAA{
						Hdr:  dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: resultTTL},
						AAAA: net.ParseIP(*(AAAA.IPv6Address)),
					}
					newZ.Insert(aaaa)
				}
			}

			if v.Properties.MxRecords != nil {
				for _, MX := range v.Properties.MxRecords {
					mx := &dns.MX{
						Hdr:        dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: resultTTL},
						Preference: uint16(*(MX.Preference)),
						Mx:         dns.Fqdn(*(MX.Exchange)),
					}
					newZ.Insert(mx)
				}
			}

			if v.Properties.PtrRecords != nil {
				for _, PTR := range v.Properties.PtrRecords {
					ptr := &dns.PTR{
						Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: resultTTL},
						Ptr: dns.Fqdn(*(PTR.Ptrdname)),
					}
					newZ.Insert(ptr)
				}
			}

			if v.Properties.SrvRecords != nil {
				for _, SRV := range v.Properties.SrvRecords {
					srv := &dns.SRV{
						Hdr:      dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: resultTTL},
						Priority: uint16(*(SRV.Priority)),
						Weight:   uint16(*(SRV.Weight)),
						Port:     uint16(*(SRV.Port)),
						Target:   dns.Fqdn(*(SRV.Target)),
					}
					newZ.Insert(srv)
				}
			}

			if v.Properties.TxtRecords != nil {
				for _, TXT := range v.Properties.TxtRecords {
					var strings []string
					for _, ptr := range TXT.Value {
						if ptr != nil {
							strings = append(strings, *ptr)
						}
					}
					txt := &dns.TXT{
						Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: resultTTL},
						Txt: strings,
					}
					newZ.Insert(txt)
				}
			}

			if v.Properties.NsRecords != nil {
				for _, NS := range v.Properties.NsRecords {
					ns := &dns.NS{
						Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: resultTTL},
						Ns:  *(NS.Nsdname),
					}
					newZ.Insert(ns)
				}
			}

			if v.Properties.SoaRecord != nil {
				SOA := v.Properties.SoaRecord
				soa := &dns.SOA{
					Hdr:     dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: resultTTL},
					Minttl:  uint32(*(SOA.MinimumTTL)),
					Expire:  uint32(*(SOA.ExpireTime)),
					Retry:   uint32(*(SOA.RetryTime)),
					Refresh: uint32(*(SOA.RefreshTime)),
					Serial:  uint32(*(SOA.SerialNumber)),
					Mbox:    dns.Fqdn(*(SOA.Email)),
					Ns:      *(SOA.Host),
				}
				newZ.Insert(soa)
			}

			if v.Properties.CnameRecord != nil {
				CNAME := v.Properties.CnameRecord.Cname
				cname := &dns.CNAME{
					Hdr:    dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: resultTTL},
					Target: dns.Fqdn(*CNAME),
				}
				newZ.Insert(cname)
			}
		}
	}
	return nil
}

func updateZoneFromPrivateResourceSet(recordSet *runtime.Pager[privatedns.RecordSetsClientListResponse], newZ *file.Zone) error {
	ctx := context.Background()
	for recordSet.More() {
		page, err := recordSet.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, v := range page.Value {
			resultFqdn := *v.Properties.Fqdn
			// TODO(vijayt): Azure TTL is int64 but below it expects uint32
			// The maximum value for the TTL can be 2,147,483,647 and
			// the maximum that a uint32 can hold is 4,294,967,295 so this should be ok but check with the maintainers
			resultTTL := uint32(*v.Properties.TTL)
			if v.Properties.ARecords != nil {
				for _, A := range v.Properties.ARecords {
					a := &dns.A{
						Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: resultTTL},
						A:   net.ParseIP(*(A.IPv4Address)),
					}
					newZ.Insert(a)
				}
			}

			if v.Properties.AaaaRecords != nil {
				for _, AAAA := range v.Properties.AaaaRecords {
					aaaa := &dns.AAAA{
						Hdr:  dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: resultTTL},
						AAAA: net.ParseIP(*(AAAA.IPv6Address)),
					}
					newZ.Insert(aaaa)
				}
			}

			if v.Properties.MxRecords != nil {
				for _, MX := range v.Properties.MxRecords {
					mx := &dns.MX{
						Hdr:        dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: resultTTL},
						Preference: uint16(*(MX.Preference)),
						Mx:         dns.Fqdn(*(MX.Exchange)),
					}
					newZ.Insert(mx)
				}
			}

			if v.Properties.PtrRecords != nil {
				for _, PTR := range v.Properties.PtrRecords {
					ptr := &dns.PTR{
						Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: resultTTL},
						Ptr: dns.Fqdn(*(PTR.Ptrdname)),
					}
					newZ.Insert(ptr)
				}
			}

			if v.Properties.SrvRecords != nil {
				for _, SRV := range v.Properties.SrvRecords {
					srv := &dns.SRV{
						Hdr:      dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: resultTTL},
						Priority: uint16(*(SRV.Priority)),
						Weight:   uint16(*(SRV.Weight)),
						Port:     uint16(*(SRV.Port)),
						Target:   dns.Fqdn(*(SRV.Target)),
					}
					newZ.Insert(srv)
				}
			}

			if v.Properties.TxtRecords != nil {
				for _, TXT := range v.Properties.TxtRecords {
					var strings []string
					for _, ptr := range TXT.Value {
						if ptr != nil {
							strings = append(strings, *ptr)
						}
					}
					txt := &dns.TXT{
						Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: resultTTL},
						Txt: strings,
					}
					newZ.Insert(txt)
				}
			}

			if v.Properties.SoaRecord != nil {
				SOA := v.Properties.SoaRecord
				soa := &dns.SOA{
					Hdr:     dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: resultTTL},
					Minttl:  uint32(*(SOA.MinimumTTL)),
					Expire:  uint32(*(SOA.ExpireTime)),
					Retry:   uint32(*(SOA.RetryTime)),
					Refresh: uint32(*(SOA.RefreshTime)),
					Serial:  uint32(*(SOA.SerialNumber)),
					Mbox:    dns.Fqdn(*(SOA.Email)),
					Ns:      *(SOA.Host),
				}
				newZ.Insert(soa)
			}

			if v.Properties.CnameRecord != nil {
				CNAME := v.Properties.CnameRecord.Cname
				cname := &dns.CNAME{
					Hdr:    dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: resultTTL},
					Target: dns.Fqdn(*CNAME),
				}
				newZ.Insert(cname)
			}
		}
	}
	return nil
}

// ServeDNS implements the plugin.Handler interface.
func (h *Azure) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	qname := state.Name()

	zone := plugin.Zones(h.zoneNames).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	zones, ok := h.zones[zone] // ok true if we are authoritative for the zone.
	if !ok || zones == nil {
		fmt.Println("SERVFAIL")
		return dns.RcodeServerFailure, nil
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	var result file.Result
	for _, z := range zones {
		h.zMu.RLock()
		m.Answer, m.Ns, m.Extra, result = z.z.Lookup(ctx, state, qname)
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
