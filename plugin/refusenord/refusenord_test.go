package refusenord

import (
	"context"
	"errors"
	"testing"

	"github.com/coredns/coredns/plugin/test"
	
	"github.com/miekg/dns"
)

type testResponseWriter struct {
	test.ResponseWriter
	Rcode int
}

func (t *testResponseWriter) setRemoteIP(ip string) {
	t.RemoteIP = ip
}

// WriteMsg implement dns.ResponseWriter interface.
func (t *testResponseWriter) WriteMsg(m *dns.Msg) error {
	t.Rcode = m.Rcode
	return nil
}

func Test_handler_ServeDNS(t *testing.T) {
	type args struct {
		rd        bool
		nextRcode int
		nextErr   error
	}
	tests := []struct {
		name      string
		args      args
		want      int
		wantErr   bool
		wantRcode int
	}{
		{
			name: "DNS query with RD set - next handler fails",
			args: args{
				true,
				dns.RcodeSuccess,
				errors.New("some error"),
			},
			want:    dns.RcodeSuccess,
			wantErr: true,
		},
		{
			name: "DNS query with RD set - next handler does not fail",
			args: args{
				true,
				dns.RcodeSuccess,
				nil,
			},
			want:    dns.RcodeSuccess,
			wantErr: false,
		},
		{
			name: "DNS query with RD not set",
			args: args{
				false,
				dns.RcodeSuccess,
				nil,
			},
			want:    dns.RcodeRefused,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := handler{
				Next: test.NextHandler(tt.args.nextRcode, tt.args.nextErr),
			}
			msg := dns.Msg{}
			msg.SetQuestion("example.com", dns.TypeA)
			msg.RecursionDesired = tt.args.rd
			w := testResponseWriter{}
			_, err := h.ServeDNS(context.Background(), &w, &msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ServeDNS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if w.Rcode != tt.want {
				t.Errorf("ServeDNS() Rcode got = %v, want %v", w.Rcode, tt.want)
			}
		})
	}
}
