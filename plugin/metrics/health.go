package metrics

// Health implements the health.Healther interface.
func (m *Metrics) Health() bool {
	return !m.uniqAddr.HasTodo()
}
