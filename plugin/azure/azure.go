package azure

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"

	publicdns "github.com/Azure/azure-sdk-for-go/profiles/latest/dns/mgmt/dns"
	privatedns "github.com/Azure/azure-sdk-for-go/profiles/latest/privatedns/mgmt/privatedns"
	"github.com/miekg/dns"
)

type zone struct {
	id      string
	z       *file.Zone
	zone    string
	private bool
}

type zones map[string][]*zone

// Azure is the core struct of the azure plugin.
type Azure struct {
	zoneNames     []string
	publicClient  publicdns.RecordSetsClient
	privateClient privatedns.RecordSetsClient
	upstream      *upstream.Upstream
	zMu           sync.RWMutex
	zones         zones
	updates       sync.WaitGroup

	Next plugin.Handler
	Fall fall.F
}

// New initializes the configured DNS zones without contacting Azure.
func New(_ctx context.Context, publicClient publicdns.RecordSetsClient, privateClient privatedns.RecordSetsClient, keys map[string][]string, accessMap map[string]string) (*Azure, error) {
	zones := make(map[string][]*zone, len(keys))
	names := make([]string, 0, len(keys))
	for resourceGroup, znames := range keys {
		for _, name := range znames {
			fqdn := dns.Fqdn(name)
			if _, ok := zones[fqdn]; !ok {
				names = append(names, fqdn)
			}
			zones[fqdn] = append(zones[fqdn], &zone{
				id: resourceGroup, zone: name, private: accessMap[resourceGroup+name] == "private",
				z: file.NewZone(fqdn, ""),
			})
		}
	}

	return &Azure{
		publicClient:  publicClient,
		privateClient: privateClient,
		zones:         zones,
		zoneNames:     names,
		upstream:      upstream.New(),
	}, nil
}

// Run starts initial and periodic zone synchronization in the background.
func (h *Azure) Run(ctx context.Context) error {
	h.updates.Go(func() {
		h.run(ctx, time.Minute)
	})
	return nil
}

func (h *Azure) run(ctx context.Context, interval time.Duration) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if ctx.Err() != nil {
				return
			}
			if err := h.updateZones(ctx); err != nil && ctx.Err() == nil {
				log.Errorf("Failed to update zones %v: %v", h.zoneNames, err)
			}
			timer.Reset(interval)
		}
	}
}

func (h *Azure) updateZones(ctx context.Context) error {
	errs := make([]string, 0)
	for zName, z := range h.zones {
		for _, hostedZone := range z {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := h.updateZone(ctx, zName, hostedZone); err != nil {
				errs = append(errs, fmt.Sprintf("failed to update %s:%s from azure: %v", hostedZone.id, hostedZone.zone, err))
			}
		}
	}

	if len(errs) != 0 {
		return fmt.Errorf("errors updating zones: %v", errs)
	}
	return nil
}

func (h *Azure) updateZone(ctx context.Context, name string, hostedZone *zone) error {
	// Bound the entire listing, including SDK retries and all pages, so a
	// failing zone cannot indefinitely prevent other zones from updating.
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	newZ := file.NewZone(name, "")
	if hostedZone.private {
		page, err := h.privateClient.List(ctx, hostedZone.id, hostedZone.zone, nil, "")
		if err != nil {
			return err
		}
		for page.NotDone() {
			updateZoneFromPrivateResourceSet(page, newZ)
			if err := page.NextWithContext(ctx); err != nil {
				return err
			}
		}
	} else {
		page, err := h.publicClient.ListByDNSZone(ctx, hostedZone.id, hostedZone.zone, nil, "")
		if err != nil {
			return err
		}
		for page.NotDone() {
			updateZoneFromPublicResourceSet(page, newZ)
			if err := page.NextWithContext(ctx); err != nil {
				return err
			}
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if newZ.SOA == nil {
		return fmt.Errorf("zone has no SOA record")
	}
	newZ.Upstream = h.upstream
	h.zMu.Lock()
	hostedZone.z = newZ
	h.zMu.Unlock()
	return nil
}

func updateZoneFromPublicResourceSet(recordSet publicdns.RecordSetListResultPage, newZ *file.Zone) {
	for _, result := range *(recordSet.Response().Value) {
		resultFqdn := *(result.Fqdn)
		resultTTL := uint32(*(result.TTL)) // #nosec G115 -- Azure API guarantees TTL fits in uint32
		if result.ARecords != nil {
			for _, A := range *(result.ARecords) {
				a := &dns.A{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: resultTTL},
					A: net.ParseIP(*(A.Ipv4Address))}
				newZ.Insert(a)
			}
		}

		if result.AaaaRecords != nil {
			for _, AAAA := range *(result.AaaaRecords) {
				aaaa := &dns.AAAA{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: resultTTL},
					AAAA: net.ParseIP(*(AAAA.Ipv6Address))}
				newZ.Insert(aaaa)
			}
		}

		if result.MxRecords != nil {
			for _, MX := range *(result.MxRecords) {
				mx := &dns.MX{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: resultTTL},
					Preference: uint16(*(MX.Preference)), // #nosec G115 -- MX preference fits in uint16
					Mx:         dns.Fqdn(*(MX.Exchange))}
				newZ.Insert(mx)
			}
		}

		if result.PtrRecords != nil {
			for _, PTR := range *(result.PtrRecords) {
				ptr := &dns.PTR{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: resultTTL},
					Ptr: dns.Fqdn(*(PTR.Ptrdname))}
				newZ.Insert(ptr)
			}
		}

		if result.SrvRecords != nil {
			for _, SRV := range *(result.SrvRecords) {
				srv := &dns.SRV{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: resultTTL},
					Priority: uint16(*(SRV.Priority)), // #nosec G115 -- SRV priority fits in uint16
					Weight:   uint16(*(SRV.Weight)),   // #nosec G115 -- SRV weight fits in uint16
					Port:     uint16(*(SRV.Port)),     // #nosec G115 -- Port fits in uint16
					Target:   dns.Fqdn(*(SRV.Target))}
				newZ.Insert(srv)
			}
		}

		if result.TxtRecords != nil {
			for _, TXT := range *(result.TxtRecords) {
				txt := &dns.TXT{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: resultTTL},
					Txt: *(TXT.Value)}
				newZ.Insert(txt)
			}
		}

		if result.NsRecords != nil {
			for _, NS := range *(result.NsRecords) {
				ns := &dns.NS{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: resultTTL},
					Ns: *(NS.Nsdname)}
				newZ.Insert(ns)
			}
		}

		if result.SoaRecord != nil {
			SOA := result.SoaRecord
			soa := &dns.SOA{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: resultTTL},
				Minttl:  uint32(*(SOA.MinimumTTL)),   // #nosec G115 -- DNS protocol mandates uint32 for SOA
				Expire:  uint32(*(SOA.ExpireTime)),   // #nosec G115 -- DNS protocol mandates uint32 for SOA
				Retry:   uint32(*(SOA.RetryTime)),    // #nosec G115 -- DNS protocol mandates uint32 for SOA
				Refresh: uint32(*(SOA.RefreshTime)),  // #nosec G115 -- DNS protocol mandates uint32 for SOA
				Serial:  uint32(*(SOA.SerialNumber)), // #nosec G115 -- DNS protocol mandates uint32 for SOA
				Mbox:    dns.Fqdn(*(SOA.Email)),
				Ns:      *(SOA.Host)}
			newZ.Insert(soa)
		}

		if result.CnameRecord != nil {
			CNAME := result.CnameRecord.Cname
			cname := &dns.CNAME{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: resultTTL},
				Target: dns.Fqdn(*CNAME)}
			newZ.Insert(cname)
		}
	}
}

func updateZoneFromPrivateResourceSet(recordSet privatedns.RecordSetListResultPage, newZ *file.Zone) {
	for _, result := range *(recordSet.Response().Value) {
		resultFqdn := *(result.Fqdn)
		resultTTL := uint32(*(result.TTL)) // #nosec G115 -- Azure API guarantees TTL fits in uint32
		if result.ARecords != nil {
			for _, A := range *(result.ARecords) {
				a := &dns.A{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: resultTTL},
					A: net.ParseIP(*(A.Ipv4Address))}
				newZ.Insert(a)
			}
		}
		if result.AaaaRecords != nil {
			for _, AAAA := range *(result.AaaaRecords) {
				aaaa := &dns.AAAA{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: resultTTL},
					AAAA: net.ParseIP(*(AAAA.Ipv6Address))}
				newZ.Insert(aaaa)
			}
		}

		if result.MxRecords != nil {
			for _, MX := range *(result.MxRecords) {
				mx := &dns.MX{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: resultTTL},
					Preference: uint16(*(MX.Preference)), // #nosec G115 -- MX preference fits in uint16
					Mx:         dns.Fqdn(*(MX.Exchange))}
				newZ.Insert(mx)
			}
		}

		if result.PtrRecords != nil {
			for _, PTR := range *(result.PtrRecords) {
				ptr := &dns.PTR{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: resultTTL},
					Ptr: dns.Fqdn(*(PTR.Ptrdname))}
				newZ.Insert(ptr)
			}
		}

		if result.SrvRecords != nil {
			for _, SRV := range *(result.SrvRecords) {
				srv := &dns.SRV{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: resultTTL},
					Priority: uint16(*(SRV.Priority)), // #nosec G115 -- SRV priority fits in uint16
					Weight:   uint16(*(SRV.Weight)),   // #nosec G115 -- SRV weight fits in uint16
					Port:     uint16(*(SRV.Port)),     // #nosec G115 -- Port fits in uint16
					Target:   dns.Fqdn(*(SRV.Target))}
				newZ.Insert(srv)
			}
		}

		if result.TxtRecords != nil {
			for _, TXT := range *(result.TxtRecords) {
				txt := &dns.TXT{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: resultTTL},
					Txt: *(TXT.Value)}
				newZ.Insert(txt)
			}
		}

		if result.SoaRecord != nil {
			SOA := result.SoaRecord
			soa := &dns.SOA{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: resultTTL},
				Minttl:  uint32(*(SOA.MinimumTTL)),   // #nosec G115 -- DNS protocol mandates uint32 for SOA
				Expire:  uint32(*(SOA.ExpireTime)),   // #nosec G115 -- DNS protocol mandates uint32 for SOA
				Retry:   uint32(*(SOA.RetryTime)),    // #nosec G115 -- DNS protocol mandates uint32 for SOA
				Refresh: uint32(*(SOA.RefreshTime)),  // #nosec G115 -- DNS protocol mandates uint32 for SOA
				Serial:  uint32(*(SOA.SerialNumber)), // #nosec G115 -- DNS protocol mandates uint32 for SOA
				Mbox:    dns.Fqdn(*(SOA.Email)),
				Ns:      dns.Fqdn(*(SOA.Host))}
			newZ.Insert(soa)
		}

		if result.CnameRecord != nil {
			CNAME := result.CnameRecord.Cname
			cname := &dns.CNAME{Hdr: dns.RR_Header{Name: resultFqdn, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: resultTTL},
				Target: dns.Fqdn(*CNAME)}
			newZ.Insert(cname)
		}
	}
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
		return dns.RcodeServerFailure, nil
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	var result file.Result
	for _, z := range zones {
		// Only the zone pointer itself needs to be guarded against a
		// concurrent swap in updateZones; Lookup can run unlocked since it
		// may block for a while resolving external names via upstream.
		h.zMu.RLock()
		zz := z.z
		h.zMu.RUnlock()

		m.Answer, m.Ns, m.Extra, result = zz.Lookup(ctx, state, qname)

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
