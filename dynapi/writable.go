package dynapi

// Writable is an interface providing
// functionality for other plugins that wish to
// implement dynamic DNS API updates.
// The `dynapirest` plugin will scan during setup
// to find plugins implementing the interface and
// provide dynamic API REST interface.
type Writable interface {
	// GetZones returns all zones handled by the implementer of the interface.
	GetZones() []string
	// Create attempts to create a dns resource record in the zone specified in `request`.
	// Returns a nil error if successful.
	Create(request *Request) error
	// Upsert attempts to update or create a dns resource record in the zone specified in `request`.
	// Returns a nil error if successful.
	Upsert(request *Request) error
	// Delete attempts to delete a dns resource record in the zone specified in `request`,
	// by exact matching all attributes.
	// Always returns nil error due to implementation specifics in `dns` package.
	Delete(request *Request) error
	// Update attempts to update a dns resource record in the zone specified in `request`.
	// Returns a nil error if successful.
	Update(request *Request) error
	// Exists checks if a dns resource record exists in the zone specified in `request`
	// by exact matching all attributes.
	Exists(request *Request) bool
	// Exists checks if a dns resource record exists in the zone specified in `request`
	// by only matching by the `name` specified in `request`.
	ExistsByName(request *Request) bool
}
