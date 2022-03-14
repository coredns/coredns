package dnsserver

import (
	"strings"
	"testing"
)

func Test_startUpZones(t *testing.T) {
	type args struct {
		protocol string
		addr     string
		zones    map[string]*Config
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Test1",
			args: args{
				protocol: "udp",
				addr:     "dns://:53",
				zones: map[string]*Config{
					"example.com": nil},
			},
			want: "udpexample.com:53",
		}, {
			name: "Test2",
			args: args{
				protocol: "udp",
				addr:     "dns://127.0.0.1:4005",
				zones: map[string]*Config{
					"example.com": nil},
			},
			want: "udpexample.com:4005 on 127.0.0.1",
		}, {

			name: "Test3",
			args: args{
				protocol: "udp",
				addr:     "dns://",
				zones: map[string]*Config{
					"example.com": nil},
			},
			want: "udpexample.com:dns://",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := startUpZones(tt.args.protocol, tt.args.addr, tt.args.zones); strings.TrimSpace(got) != tt.want {
				t.Errorf("startUpZones() = %v, want %v", got, tt.want)
			}
		})
	}
}
