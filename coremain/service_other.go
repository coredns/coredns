//go:build !windows

package coremain

import "github.com/coredns/caddy"

func runService(instance *caddy.Instance) {
	instance.Wait()
}
