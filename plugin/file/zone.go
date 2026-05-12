package file

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin/file/tree"
	"github.com/coredns/coredns/plugin/pkg/upstream"

	"github.com/miekg/dns"
)

// Zone is a structure that contains all data related to a DNS zone.
type Zone struct {
	origin  string
	origLen int
	file    string
	*tree.Tree
	Apex
	Expired bool

	sync.RWMutex

	StartupOnce  sync.Once
	TransferFrom []string

	ReloadInterval time.Duration
	reloadShutdown chan bool

	Upstream *upstream.Upstream // Upstream for looking up external names during the resolution process.
}

// Apex contains the apex records of a zone: SOA, NS and their potential signatures.
type Apex struct {
	SOA    *dns.SOA
	NS     []dns.RR
	SIGSOA []dns.RR
	SIGNS  []dns.RR
}

// NewZone returns a new zone.
func NewZone(name, file string) *Zone {
	return &Zone{
		origin:         dns.Fqdn(name),
		origLen:        dns.CountLabel(dns.Fqdn(name)),
		file:           filepath.Clean(file),
		Tree:           &tree.Tree{},
		reloadShutdown: make(chan bool),
	}
}

// Copy copies a zone.
func (z *Zone) Copy() *Zone {
	z1 := NewZone(z.origin, z.file)
	z1.TransferFrom = z.TransferFrom

	z.RLock()
	z1.Expired = z.Expired
	z1.Apex = z.Apex
	z.RUnlock()

	return z1
}

// CopyWithoutApex copies zone z without the Apex records.
func (z *Zone) CopyWithoutApex() *Zone {
	z1 := NewZone(z.origin, z.file)
	z1.TransferFrom = z.TransferFrom

	z.RLock()
	z1.Expired = z.Expired
	z.RUnlock()

	return z1
}

// Insert inserts r into z.
func (z *Zone) Insert(r dns.RR) error {
	// r.Header().Name = strings.ToLower(r.Header().Name)
	if r.Header().Rrtype != dns.TypeSRV {
		r.Header().Name = strings.ToLower(r.Header().Name)
	}

	switch h := r.Header().Rrtype; h {
	case dns.TypeNS:
		r.(*dns.NS).Ns = strings.ToLower(r.(*dns.NS).Ns)

		if r.Header().Name == z.origin {
			z.NS = append(z.NS, r)
			return nil
		}
	case dns.TypeSOA:
		r.(*dns.SOA).Ns = strings.ToLower(r.(*dns.SOA).Ns)
		r.(*dns.SOA).Mbox = strings.ToLower(r.(*dns.SOA).Mbox)

		z.SOA = r.(*dns.SOA)
		return nil
	case dns.TypeNSEC3, dns.TypeNSEC3PARAM:
		return fmt.Errorf("NSEC3 zone is not supported, dropping RR: %s for zone: %s", r.Header().Name, z.origin)
	case dns.TypeRRSIG:
		x := r.(*dns.RRSIG)
		switch x.TypeCovered {
		case dns.TypeSOA:
			z.SIGSOA = append(z.SIGSOA, x)
			return nil
		case dns.TypeNS:
			if r.Header().Name == z.origin {
				z.SIGNS = append(z.SIGNS, x)
				return nil
			}
		}
	case dns.TypeCNAME:
		r.(*dns.CNAME).Target = strings.ToLower(r.(*dns.CNAME).Target)
	case dns.TypeMX:
		r.(*dns.MX).Mx = strings.ToLower(r.(*dns.MX).Mx)
	case dns.TypeSRV:
		// r.(*dns.SRV).Target = strings.ToLower(r.(*dns.SRV).Target)
	case dns.TypeSVCB:
		r.(*dns.SVCB).Target = strings.ToLower(r.(*dns.SVCB).Target)
	case dns.TypeHTTPS:
		r.(*dns.HTTPS).Target = strings.ToLower(r.(*dns.HTTPS).Target)
	}

	z.Tree.Insert(r)
	return nil
}

// File retrieves the file path in a safe way.
func (z *Zone) File() string {
	z.RLock()
	defer z.RUnlock()
	return z.file
}

// SetFile updates the file path in a safe way.
func (z *Zone) SetFile(path string) {
	z.Lock()
	z.file = path
	z.Unlock()
}

// snapshot returns the apex and tree under a single read lock so callers see
// a consistent zone generation even if TransferIn or Reload swaps them.
func (z *Zone) snapshot() (Apex, *tree.Tree) {
	z.RLock()
	defer z.RUnlock()
	return z.Apex, z.Tree
}

// setData atomically replaces the zone's apex and tree and clears the expired
// flag. It is the write-side counterpart to snapshot.
func (z *Zone) setData(ap Apex, t *tree.Tree) {
	z.Lock()
	z.Apex = ap
	z.Tree = t
	z.Expired = false
	z.Unlock()
}

// records returns the apex records in zone-file order (SOA, RRSIG(SOA), NS,
// RRSIG(NS)), or an error if no SOA is set.
func (a Apex) records() ([]dns.RR, error) {
	if a.SOA == nil {
		return nil, fmt.Errorf("no SOA")
	}
	rrs := make([]dns.RR, 0, 1+len(a.SIGSOA)+len(a.NS)+len(a.SIGNS))
	rrs = append(rrs, a.SOA)
	rrs = append(rrs, a.SIGSOA...)
	rrs = append(rrs, a.NS...)
	rrs = append(rrs, a.SIGNS...)
	return rrs, nil
}

// ApexIfDefined returns the apex nodes from z. The SOA record is the first record, if it does not exist, an error is returned.
func (z *Zone) ApexIfDefined() ([]dns.RR, error) {
	ap, _ := z.snapshot()
	return ap.records()
}

// NameFromRight returns the labels from the right, staring with the
// origin and then i labels extra. When we are overshooting the name
// the returned boolean is set to true.
func (z *Zone) nameFromRight(qname string, i int) (string, bool) {
	if i <= 0 {
		return z.origin, false
	}

	n := len(qname)
	for j := 1; j <= z.origLen; j++ {
		m, shot := dns.PrevLabel(qname[:n], 1)
		if shot {
			return qname, shot
		}
		n = m
	}

	for j := 1; j <= i; j++ {
		m, shot := dns.PrevLabel(qname[:n], 1)
		if shot {
			return qname, shot
		}
		n = m
	}
	return qname[n:], false
}

func (z *Zone) getSOA() *dns.SOA {
	z.RLock()
	defer z.RUnlock()
	return z.SOA
}
