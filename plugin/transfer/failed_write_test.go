package transfer

import (
	"context"
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

type badwriter struct {
	dns.ResponseWriter
	count int
}

func (w *badwriter) WriteMsg(res *dns.Msg) error { return fmt.Errorf("failed to write msg") }

func TestWriteMessageFailed(t *testing.T) {
	transfer := newTestTransfer()
	ctx := context.TODO()
	w := &badwriter{ResponseWriter: &test.ResponseWriter{TCP: true}}
	m := &dns.Msg{}
	m.SetAxfr("example.org.")

	_, err := transfer.ServeDNS(ctx, w, m)
	if err == nil {
		t.Error("Expected error, got none")
	}
}
