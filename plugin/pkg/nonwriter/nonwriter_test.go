package nonwriter

import (
	"fmt"
	"testing"

	"github.com/miekg/dns"
)

func TestNonWriter(t *testing.T) {
	nw := New(nil)
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	if err := nw.WriteMsg(m); err != nil {
		t.Errorf("Got error when writing to nonwriter: %s", err)
	}
	if x := nw.Msg.Question[0].Name; x != "example.org." {
		t.Errorf("Expacted 'example.org.' got %q:", x)
	}
}

func TestNonWriterMultipleWrites(t *testing.T) {
	nw := New(nil)
	for i := range 3 {
		m := new(dns.Msg)
		m.SetQuestion(fmt.Sprintf("%d.example.org.", i), dns.TypeA)
		if err := nw.WriteMsg(m); err != nil {
			t.Fatalf("Got error when writing to nonwriter: %s", err)
		}
	}

	if len(nw.Msgs) != 3 {
		t.Fatalf("Expected 3 messages, but got %d", len(nw.Msgs))
	}
	for i, m := range nw.Msgs {
		if x := m.Question[0].Name; x != fmt.Sprintf("%d.example.org.", i) {
			t.Errorf("Expected message %d to be %q, but got %q", i, fmt.Sprintf("%d.example.org.", i), x)
		}
	}
	if x := nw.Msg.Question[0].Name; x != "2.example.org." {
		t.Errorf("Expected Msg to hold the last message, but got %q", x)
	}
}

func TestNonWriterSingleWrite(t *testing.T) {
	nw := New(nil)
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	if err := nw.WriteMsg(m); err != nil {
		t.Fatalf("Got error when writing to nonwriter: %s", err)
	}
	if nw.Msgs != nil {
		t.Errorf("Expected Msgs to stay nil for a single write, but got %d messages", len(nw.Msgs))
	}
}
