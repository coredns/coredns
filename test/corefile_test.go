package test

import (
	"testing"
)

func TestCorefile1(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Expected no panic, but got %v", r)
		}
	}()

	// this used to crash
	corefile := `\\\\ȶ.
acl
`
	i, _, _, _ := CoreDNSServerAndPorts(corefile)
	defer func() {
		if i != nil {
			i.Stop()
		}
	}()
}
