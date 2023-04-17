//go:build exclude

package main

import "github.com/coredns/coredns/plugin/atlas/cli/cmd"

func main() {
	cmd.Execute()
}
