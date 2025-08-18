package proxy

import (
	"errors"
)

var (
	// ErrNoHealthy means no healthy proxies left.
	ErrNoHealthy = errors.New("no healthy proxies")
	// ErrNoForward means no forwarder defined.
	ErrNoForward = errors.New("no forwarder defined")
	// ErrCachedClosed means cached connection was closed by peer.
	ErrCachedClosed = errors.New("cached connection was closed by peer")
	// ErrFormatError means that the dns server returned a format error
	ErrFormatError = errors.New("dns format error")
	// ErrServerFailure means that the dns server failed to process the request
	ErrServerFailure = errors.New("server failure")
	// ErrNotImplemented means that the dns server not implemented the requested type
	ErrNotImplemented = errors.New("not implemented ")
	// ErrRefused means that the dns server refused the request
	ErrRefused = errors.New("refused")
)

// Options holds various Options that can be set.
type Options struct {
	// ForceTCP use TCP protocol for upstream DNS request. Has precedence over PreferUDP flag
	ForceTCP bool
	// PreferUDP use UDP protocol for upstream DNS request.
	PreferUDP bool
	// HCRecursionDesired sets recursion desired flag for Proxy healthcheck requests
	HCRecursionDesired bool
	// HCDomain sets domain for Proxy healthcheck requests
	HCDomain string
}

// RcodeToError converts a DNS response code to an error.
func RcodeToError(rc int) error {
	switch rc {
	case 1:
		return ErrFormatError
	case 2:
		return ErrServerFailure
	case 4:
		return ErrNotImplemented
	case 5:
		return ErrRefused
	default:
		return nil
	}
}
