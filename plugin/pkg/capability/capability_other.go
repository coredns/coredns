//go:build !linux

package capability

// WithNetBindService calls listen without changing privileges on non-Linux systems.
func WithNetBindService[T any](listen func() (T, error)) (T, error) {
	return listen()
}
