package ecs

// Ready implements the ready.Readiness interface, once this flips to true CoreDNS
// assumes this plugin is ready for queries; it is not checked again.
func (e Ecs) Ready() bool { return true }
