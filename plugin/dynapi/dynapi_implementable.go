package dynapi

type (
	DynapiRequest struct {
		Zone string `json:"zone"`
		Name string `json:"name"`
		// NOTE: Can be 'A' or 'AAAA' record type.
		Type string `json:"type"`
		// NOTE: IPv4 or IPv6.
		Address string `json:"address"`
	}

	DynapiImplementable interface {
		GetZones() []string
		// Create returns an error. If it's nil, it's a successful creation.
		Create(request *DynapiRequest) error
		// Delete returns an error. If it's nil, it's a successful deletion.
		Delete(request *DynapiRequest) error
		// Update returns an error. If it's nil, it's a successful update.
		Update(request *DynapiRequest) error
	}
)
