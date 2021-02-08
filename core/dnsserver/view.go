package dnsserver

import "github.com/coredns/coredns/request"

type Viewer interface {
	Filter(request.Request) bool
}