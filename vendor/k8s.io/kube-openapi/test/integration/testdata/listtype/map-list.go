package listtype

// +k8s:openapi-gen=true
type MapList struct {
	// +listType=map
	// +listMapKey=port
	Field []Item
}

// +k8s:openapi-gen=true
type Item struct {
	Protocol string
	Port     int
}
