package secondary

import (
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/catalog"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/plugin/transfer"

	"github.com/miekg/dns"
)

func (s *Secondary) transferIn(origin string, z *file.Zone, t *transfer.Transfer) error {
	if _, ok := s.catalogZones[origin]; !ok {
		return z.TransferIn(t)
	}

	var parsed *catalog.Catalog
	if err := z.TransferInWithRecords(t, func(rrs []dns.RR) error {
		cat, err := catalog.Parse(origin, rrs)
		if err != nil {
			return err
		}
		parsed = cat
		return nil
	}); err != nil {
		return err
	}
	if parsed == nil {
		return nil
	}

	s.catalogMu.Lock()
	if s.catalogs == nil {
		s.catalogs = make(map[string]*catalog.Catalog)
	}
	s.catalogs[origin] = parsed
	s.catalogMu.Unlock()

	s.applyCatalog(origin, parsed, z, t)
	log.Infof("Parsed catalog zone %s with %d member zones", origin, len(parsed.Members))
	return nil
}

type dynamicZoneStart struct {
	origin   string
	zone     *file.Zone
	shutdown chan bool
}

func (s *Secondary) applyCatalog(origin string, cat *catalog.Catalog, catalogZone *file.Zone, t *transfer.Transfer) {
	memberZones := make(map[string]struct{}, len(cat.Members))
	var starts []dynamicZoneStart

	s.zoneMu.Lock()
	s.ensureZoneStateLocked()

	for _, member := range cat.Members {
		memberZones[member.Zone] = struct{}{}

		if existing, ok := s.Z[member.Zone]; ok {
			if dyn, ok := s.dynamicZones[member.Zone]; ok && dyn.catalog == origin && existing != nil {
				continue
			}
			log.Warningf("Skipping catalog member zone %s from %s: zone already exists", member.Zone, origin)
			continue
		}

		z := file.NewZone(member.Zone, "stdin")
		if catalogZone != nil {
			z.TransferFrom = append([]string(nil), catalogZone.TransferFrom...)
		}
		z.Upstream = upstream.New()

		shutdown := make(chan bool)
		s.Z[member.Zone] = z
		s.Names = append(s.Names, member.Zone)
		s.zoneNames[z] = member.Zone
		s.dynamicZones[member.Zone] = &dynamicZone{catalog: origin, shutdown: shutdown}
		starts = append(starts, dynamicZoneStart{origin: member.Zone, zone: z, shutdown: shutdown})
		log.Infof("Added catalog member zone %s from catalog %s", member.Zone, origin)
	}

	for member := range s.catalogMemberZones[origin] {
		if _, ok := memberZones[member]; ok {
			continue
		}
		dyn, ok := s.dynamicZones[member]
		if !ok || dyn.catalog != origin {
			continue
		}
		dyn.stopOnce.Do(func() { close(dyn.shutdown) })
		delete(s.dynamicZones, member)
		if z := s.Z[member]; z != nil {
			delete(s.zoneNames, z)
		}
		delete(s.Z, member)
		s.Names = removeZoneName(s.Names, member)
		log.Infof("Removed catalog member zone %s from catalog %s", member, origin)
	}
	s.catalogMemberZones[origin] = memberZones
	s.zoneMu.Unlock()

	for _, start := range starts {
		go s.transferAndUpdate(start.origin, start.zone, t, start.shutdown)
	}
}

func (s *Secondary) ensureZoneStateLocked() {
	if s.Z == nil {
		s.Z = make(map[string]*file.Zone)
	}
	if s.zoneNames == nil {
		s.zoneNames = make(map[*file.Zone]string, len(s.Z))
		for name, zone := range s.Z {
			if zone != nil {
				s.zoneNames[zone] = name
			}
		}
	}
	if s.dynamicZones == nil {
		s.dynamicZones = make(map[string]*dynamicZone)
	}
	if s.catalogMemberZones == nil {
		s.catalogMemberZones = make(map[string]map[string]struct{})
	}
}
