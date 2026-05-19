//go:build coredns_all || coredns_kubernetes || coredns_k8s_external

package kubernetes

// Ready implements the ready.Readiness interface.
func (k *Kubernetes) Ready() bool { return k.APIConn.HasSynced() }
