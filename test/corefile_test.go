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
	i, _, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Expected no error, but got %v", err)
	}
	defer i.Stop()
}
