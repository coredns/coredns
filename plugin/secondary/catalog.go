package secondary

import (
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/catalog"
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
	log.Infof("Parsed catalog zone %s with %d member zones", origin, len(parsed.Members))
	return nil
}
