//go:build !windows

package main

//go:generate go run directives_generate.go
//go:generate go run owners_generate.go

import (
	"flag"

	_ "github.com/coredns/coredns/core/plugin" // Plug in CoreDNS.
	"github.com/coredns/coredns/coremain"
)

func main() {
	flag.Parse()
	coremain.RunForever()
}
