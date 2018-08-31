package dnssec

import (
	"bytes"

	"github.com/miekg/dns"
)

// hash serializes the RRset and return a signature cache key.
func hash(rrs []dns.RR) string {
	h := bytes.Buffer{}
	buf := make([]byte, 256)
	for _, r := range rrs {
		off, err := dns.PackRR(r, buf, 0, nil, false)
		if err == nil {
			h.Write(buf[:off])
		}
	}

	return h.String()
}
