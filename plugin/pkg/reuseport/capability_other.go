//go:build !linux

package reuseport

func listenWithBindCapability[T any](listen func() (T, error)) (T, error) {
	return listen()
}
