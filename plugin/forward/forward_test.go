package forward

import (
	"testing"
)

func TestList(t *testing.T) {
	f := Forward{
		proxies: []*Proxy{{addr: "1.1.1.1:53"}, {addr: "2.2.2.2:53"}, {addr: "3.3.3.3:53"}},
		p:       &roundRobin{},
	}

	expect := []*Proxy{{addr: "2.2.2.2:53"}, {addr: "1.1.1.1:53"}, {addr: "3.3.3.3:53"}}
	got := f.List()

	if len(got) != len(expect) {
		t.Fatalf("Expected: %v results, got: %v", len(expect), len(got))
	}
	for i, p := range got {
		if p.addr != expect[i].addr {
			t.Fatalf("Expected proxy %v to be '%v', got: '%v'", i, expect[i].addr, p.addr)
		}
	}
}
