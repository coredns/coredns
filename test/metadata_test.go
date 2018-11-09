package test

import (
	"bytes"
	"testing"

	"github.com/miekg/dns"
)

func TestMetadata(t *testing.T) {
	t.Parallel()
	corefile := `.:0 {
       metadata
  	   rewrite edns0 local set 0xffee {metadata/qname}
       erratic . {
	drop 0
	}
}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	defer i.Stop()

	testMeta(t, udp)

}

func testMeta(t *testing.T, server string) {
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)

	r, err := dns.Exchange(m, server)
	if err != nil {
		t.Fatalf("Expected to receive reply, but didn't: %s", err)
	}

	o := r.IsEdns0()
	if o == nil || len(o.Option) == 0 {
		t.Error("Expected EDNS0 options but got none")
	} else {
		if e, ok := o.Option[0].(*dns.EDNS0_LOCAL); ok {
			if e.Code != 0xffee {
				t.Errorf("Expected EDNS_LOCAL code 0xffee but got %x", e.Code)
			}
			if !bytes.Equal(e.Data, []byte("example.com.")) {
				t.Errorf("Expected EDNS_LOCAL data 'example.com.' but got %q", e.Data)
			}
		} else {
			t.Errorf("Expected EDNS0_LOCAL but got %v", o.Option[0])
		}
	}
}
