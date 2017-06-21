package backend

import (
	"github.com/coredns/coredns/middleware"
	"github.com/coredns/coredns/middleware/backend/msg"
	"github.com/coredns/coredns/middleware/proxy"
)

// StubServiceBackend is a ServiceBackend that also support stubs.
type StubServiceBackend interface {
	middleware.ServiceBackend

	Records(name string, exact bool) ([]msg.Service, error)
}

// Backend is a list of required information for the default implementation.
type Backend struct {
	Next           middleware.Handler
	Zones          []string
	ServiceName    string
	Stubmap        *map[string]proxy.Proxy // list of proxies for stub resolving.
	Debugging      bool                    // Do we allow debug queries.
	ServiceBackend StubServiceBackend
}
