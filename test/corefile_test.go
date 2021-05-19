package test

import (
	"testing"
)

func TestCorefile1(t *testing.T) {
	corefile := `ȶ
acl
`
	// this crashed, now it should return an error.
	i, _, _, err := CoreDNSServerAndPorts(corefile)
	if err == nil {
		defer i.Stop()
		t.Fatalf("Expected an error got none")
	}
}
