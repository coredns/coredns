package autopath

import (
	"github.com/coredns/coredns/middleware"
)

// Writer implements a ResponseWriter that also does the following:
// * reverts question section of a packet to its original state.
//   This is done to avoid the 'Question section mismatch:' error in client.
// * Defers write to the client if the response code is NXDOMAIN.  This is needed
//   to enable further search path probing if a search was not sucessful.
// * Allow forced write to client regardless of response code.  This is needed to
//   write the packet to the client if the final search in the path fails to
//   produce results.
// * Overwrites response code with AutoPathWriter.Rcode if the response code
//   is NXDOMAIN (NameError).  This is needed to support the AutoPath.OnNXDOMAIN
//   function, which returns a NOERROR to client instead of NXDOMAIN if the final
//   search in the path fails to produce results.

type AutoPath struct {
	Next    middleware.Handler
	Origins []string

	SearchPath []string // Search path domains to use.
	Ndots      int
	Response   int
}
