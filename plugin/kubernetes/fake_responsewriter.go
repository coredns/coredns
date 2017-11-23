package kubernetes

import (
	"net"
	"sync"

	"github.com/miekg/dns"
)

// Simple implementation of dns.ResponseWriter so we can store the dns.Msg results
// and not respond to the client
type fakewriter struct {
	sync.Mutex
	Msg      *dns.Msg
	RemoteIP net.Addr
}

func (w *fakewriter) Close() error                  { return nil }
func (w *fakewriter) TsigStatus() error             { return nil }
func (w *fakewriter) TsigTimersOnly(b bool)         { return }
func (w *fakewriter) Hijack()                       { return }
func (w *fakewriter) LocalAddr() (la net.Addr)      { return }
func (w *fakewriter) RemoteAddr() net.Addr          { return w.RemoteIP }
func (w *fakewriter) Write(buf []byte) (int, error) { return len(buf), nil }

// Need some intelligence here so we can buffer the entire response
func (w *fakewriter) WriteMsg(m *dns.Msg) error {
	w.Lock()
	if w.Msg == nil {
		w.Msg = m
	} else {
		w.Msg.Answer = append(w.Msg.Answer, m.Answer...)
	}
	w.Unlock()
	return nil
}

func (w *fakewriter) ReadMsg() *dns.Msg {
	w.Lock()
	defer w.Unlock()
	return w.Msg
}
