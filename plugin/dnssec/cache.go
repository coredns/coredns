package dnssec

import (
	"hash/fnv"
	"io"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

// hash serializes the RRset and returns a signature cache key.
func hash(rrs []dns.RR) uint64 {
	h := fnv.New64()
	// Only need this to be unique for ownername + qtype (+class), but we
	// only care about IN. Its already an RRSet, so the ownername is the
	// same as is the qtype. Take the first one and construct the hash
	// string that creates the key
	io.WriteString(h, strings.ToLower(rrs[0].Header().Name))
	typ, ok := dns.TypeToString[rrs[0].Header().Rrtype]
	if !ok {
		typ = "TYPE" + strconv.FormatUint(uint64(rrs[0].Header().Rrtype), 10)
	}
	io.WriteString(h, typ)
	i := h.Sum64()
	return i
}
