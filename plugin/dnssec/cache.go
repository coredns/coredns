package dnssec

import (
	"hash/fnv"
	"io"
	"time"

	"github.com/coredns/coredns/plugin/pkg/cache"

	"github.com/miekg/dns"
)

// hash serializes the RRset and returns a signature cache key.
func hash(rrs []dns.RR) uint64 {
	h := fnv.New64()
	// we need to hash the entire RRset to pick the correct sig, if the rrset
	// changes for whatever reason we should resign.
	// We could use wirefmt, or the string format, both create garbage when creating
	// the hash key. And of course is a uint64 big enough?
	for _, rr := range rrs {
		io.WriteString(h, rr.String())
	}
	return h.Sum64()
}

func periodicClean(c *cache.Cache, stop <-chan struct{}) {
	tick := time.NewTicker(8 * time.Hour)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			// we sign for 8 days, check if a signature in the cache reached 75% of that (i.e. 6), if found delete
			// the signature
			is75 := time.Now().UTC().Add(twoDays)
			c.Walk(func(items map[uint64]interface{}, key uint64) bool {
				for _, rr := range items[key].([]dns.RR) {
					if !rr.(*dns.RRSIG).ValidityPeriod(is75) {
						delete(items, key)
					}
				}
				return true
			})

		case <-stop:
			return
		}
	}
}
