package multicluster

// Ready implements the ready.Readiness interface.
//func (m *MultiCluster) Ready() bool { return m.controller.HasSynced() }
func (m *MultiCluster) Ready() bool { return true }
