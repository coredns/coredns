// Package secondary implements a secondary plugin.
package secondary

import (
	"sync"

	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/catalog"
)

// Secondary implements a secondary plugin that allows CoreDNS to retrieve (via AXFR)
// zone information from a primary server.
type Secondary struct {
	file.File

	catalogMu    sync.RWMutex
	catalogs     map[string]*catalog.Catalog
	catalogZones map[string]struct{}
}

// Name implements the Handler interface.
func (s *Secondary) Name() string { return "secondary" }
