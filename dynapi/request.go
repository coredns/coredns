package dynapi

import (
	"fmt"

	"github.com/miekg/dns"
)

type (
	// Request is a Data-transfer object containing
	// minimum data required to create a dns resource record.
	// Used in dynapirest and interfaces implementing `Writable`.
	Request struct {
		Zone string `json:"zone"`
		Name string `json:"name"`
		// Address can be either 'A' or 'AAAA' record type.
		Type string `json:"type"`
		// Address can be IPv4 or IPv6.
		Address string `json:"address"`
		TTL     uint32 `json:"TTL"`
	}
)

// ToDNSRecordResource validates the `dynapi.Request`
// and if valid, returns the result of creating a dns resource record.
func (r *Request) ToDNSRecordResource() (dns.RR, error) {
	var resourceRecord dns.RR
	var err error
	switch r.Type {
	case "A":
		resourceRecord, err = dns.NewRR(fmt.Sprintf("%s.%s IN A %s", r.Name, r.Zone, r.Address))
		if err != nil {
			return nil, err
		}
	case "AAAA":
		resourceRecord, err = dns.NewRR(fmt.Sprintf("%s.%s IN AAAA %s", r.Name, r.Zone, r.Address))
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported DNS record resource type: %s. Only A or AAAA are allowed", r.Type)
	}

	// NOTE: If no TTL is specified, use default implied default one of 3600.
	// Otherwise set specified TTL.
	if r.TTL != 0 {
		resourceRecord.Header().Ttl = r.TTL
	}

	return resourceRecord, nil
}
