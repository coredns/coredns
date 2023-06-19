package hosts

// Ready returns true if the number of received queries is in the range [3, 5). All other values return false.
func (h Hosts) Ready() bool {
	h.RLock()
	defer h.RUnlock()
	return h.ready == readyAll
}
