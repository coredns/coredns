package listener

import (
	"github.com/mholt/caddy"
	"log"
	"net"
)

// LsnProvider : a tool to provide listeners for TCP or Unix
var LsnProvider = provider{listeners: map[string]net.Listener{}}

type provider struct {
	// when start listening organize per key to not allow reuse of address on several listeners
	//   - check if reallocate in the per-address
	//   - allocate in the per-key
	// after service is started, subscript to onRestart
	// on Restart - then push all allocate in per-address for next restart
	//
	// if restart fails : => there will be onRestart on the same instance. So be prepared to have the sequence run several times
	//
	inuse     map[string]net.Listener
	listeners map[string]net.Listener
}

// AllocateListener : provide an access to expected listener
func (l *provider) AllocateListener(key string, network string, address string) (net.Listener, error) {
	old, ok := l.listeners[key]
	var ln net.Listener
	var err error
	if ok {
		addr := old.Addr()
		if addr.String() == address && addr.Network() == network {
			// we want to reuse the same address - duplicate the FD and open a listener on that FD
			// If this is a reload and s is a GracefulServer,
			// reuse the listener for a graceful restart.
			if fileLn, ok := old.(caddy.Listener); ok {
				file, err := fileLn.File()
				if err != nil {
					return nil, err
				}
				ln, err = net.FileListener(file)
				if err != nil {
					return nil, err
				}
				err = file.Close()
				if err != nil {
					return nil, err
				}

			}
		}
	}
	if ln == nil {
		ln, err = net.Listen(network, address)
		if err != nil {
			return nil, err
		}
	}
	l.listeners[key] = ln
	log.Printf("[INFO] %s - listening on network %s - address %s", key, ln.Addr().Network(), ln.Addr().String())
	return ln, err
}
