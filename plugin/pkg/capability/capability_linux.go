package capability

import (
	"errors"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

// WithNetBindService first tries listening without changing privileges.
// If permission is denied, it retries with CAP_NET_BIND_SERVICE when that
// capability is permitted but not effective. This also respects the network
// namespace's privileged port range without reading or changing its sysctl.
func WithNetBindService[T any](listen func() (T, error)) (T, error) {
	listener, err := listen()
	if !errors.Is(err, syscall.EACCES) {
		return listener, err
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		// Linux capabilities belong to OS threads. Keep the retry on its own
		// thread and deliberately do not unlock it: when this goroutine exits,
		// the runtime terminates the thread instead of reusing its privileges
		// for another goroutine. This works with both pure Go and cgo builds.
		runtime.LockOSThread()
		header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
		var caps [2]unix.CapUserData
		if unix.Capget(&header, &caps[0]) != nil {
			return
		}
		const bindCapability = uint32(1 << unix.CAP_NET_BIND_SERVICE)
		if caps[0].Permitted&bindCapability == 0 || caps[0].Effective&bindCapability != 0 {
			return
		}
		caps[0].Effective |= bindCapability
		if unix.Capset(&header, &caps[0]) != nil {
			return
		}
		listener, err = listen()
	}()
	<-done
	return listener, err
}
