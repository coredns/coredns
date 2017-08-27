package kubernetes

import (
	"fmt"
	"testing"
)

func TestAxfr(t *testing.T) {
	k := New([]string{"cluster.local."})
	k.APIConn = &APIConnServeTest{}

	x := NewXfr(k)
	rrs := x.All("example.org.")
	fmt.Println("\n** NEXT **")
	for _, rr := range rrs {
		fmt.Printf("%s\n", rr)
	}
}
