package secondary

import (
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
)

func (s *Secondary) lookupZone(qname string) (string, *file.Zone, bool) {
	s.zoneMu.RLock()
	defer s.zoneMu.RUnlock()

	zone := plugin.Zones(s.Names).Matches(qname)
	if zone == "" {
		return "", nil, false
	}
	z, ok := s.Z[zone]
	if !ok {
		return zone, nil, true
	}
	return zone, z, true
}

func (s *Secondary) zoneName(z *file.Zone) string {
	s.zoneMu.RLock()
	defer s.zoneMu.RUnlock()

	return s.zoneNames[z]
}

func (s *Secondary) stopDynamicZones() {
	s.zoneMu.Lock()
	defer s.zoneMu.Unlock()

	for _, dyn := range s.dynamicZones {
		dyn.stopOnce.Do(func() { close(dyn.shutdown) })
	}
}

func removeZoneName(names []string, name string) []string {
	for i, n := range names {
		if n == name {
			return append(names[:i], names[i+1:]...)
		}
	}
	return names
}
