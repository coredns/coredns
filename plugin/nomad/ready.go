package nomad

// Ready signals when the plugin is ready for use.
// In case of Nomad, when the ping to the Nomad API is successful
// the plugin is ready.
func (n Nomad) Ready() bool {
	client, _ := n.getClient()
	return client != nil
}
