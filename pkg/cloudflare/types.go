package cloudflare

// Zone represents a Cloudflare DNS zone.
type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Record represents a Cloudflare DNS record.
type Record struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"priority,omitempty"`
	Proxied  *bool  `json:"proxied,omitempty"`
}

// SyncResult summarizes a zone sync operation.
type SyncResult struct {
	Created int
	Updated int
	Deleted int
	Errors  []error
}

// recordKey returns a dedup key for diffing.
func (r Record) recordKey() string {
	return r.Type + ":" + r.Name
}

// CF API response wrappers.

type apiResponse[T any] struct {
	Success    bool       `json:"success"`
	Errors     []apiError `json:"errors"`
	Result     T          `json:"result"`
	ResultInfo *pageInfo  `json:"result_info,omitempty"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e apiError) Error() string { return e.Message }

type pageInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
	Count      int `json:"count"`
	TotalCount int `json:"total_count"`
}
