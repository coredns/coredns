package metrics

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

type inmemoryWriter struct {
	test.ResponseWriter
	written []byte
}

func (r *inmemoryWriter) WriteMsg(m *dns.Msg) error {
	r.written, _ = m.Pack()
	return r.ResponseWriter.WriteMsg(m)
}

func (r *inmemoryWriter) Write(buf []byte) (int, error) {
	r.written = buf
	return r.ResponseWriter.Write(buf)
}

func TestRecorder_WriteMsg(t *testing.T) {
	successResp := dns.Msg{}
	successResp.Answer = []dns.RR{
		test.A("a.example.org. 	1800	IN	A 127.0.0.53"),
	}

	nxdomainResp := dns.Msg{}
	nxdomainResp.Rcode = dns.RcodeNameError

	tests := []struct {
		name string
		msg  *dns.Msg
	}{
		{
			name: "should record successful response",
			msg:  &successResp,
		},
		{
			name: "should record nxdomain response",
			msg:  &nxdomainResp,
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tw := inmemoryWriter{ResponseWriter: test.ResponseWriter{}}
			rec := NewRecorder(&tw)

			if err := rec.WriteMsg(tt.msg); err != nil {
				t.Errorf("Test %d: WriteMsg() unexpected error %v", i, err)
			}

			if rec.Msg != tt.msg {
				t.Errorf("Test %d: Expected value %v for msg, but got %v", i, tt.msg, rec.Msg)
			}
			if rec.Len != tt.msg.Len() {
				t.Errorf("Test %d: Expected value %d for len, but got %d", i, tt.msg.Len(), rec.Len)
			}
			if rec.Rcode != tt.msg.Rcode {
				t.Errorf("Test %d: Expected value %d for rcode, but got %d", i, tt.msg.Rcode, rec.Rcode)
			}
		})
	}
}
