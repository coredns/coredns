package dnsserver

import (
	"github.com/coredns/caddy"
	"testing"
)

func Test_keyForConfig(t *testing.T) {
	type args struct {
		blocIndex    int
		blocKeyIndex int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Test #1",
			args: args{
				blocIndex:    0,
				blocKeyIndex: 0,
			},
			want: "0:0",
		},
		{
			name: "Test #2",
			args: args{
				blocIndex:    0,
				blocKeyIndex: 1,
			},
			want: "0:1",
		},
		{
			name: "Test #3",
			args: args{
				blocIndex:    1,
				blocKeyIndex: 0,
			},
			want: "1:0",
		},
		{
			name: "Test #4",
			args: args{
				blocIndex:    1,
				blocKeyIndex: 1,
			},
			want: "1:1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyForConfig(tt.args.blocIndex, tt.args.blocKeyIndex); got != tt.want {
				t.Errorf("keyForConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetConfig(t *testing.T) {
	type args struct {
		keyIndex   int
		BlockIndex int
	}
	tests := []struct {
		name string
		args args
		want *Config
	}{
		{
			name: "Test #1",
			args: args{
				keyIndex:   0,
				BlockIndex: 0,
			},
			want: &Config{
				Port: "Root",
			},
		},
		{
			name: "Test #2",
			args: args{
				keyIndex:   0,
				BlockIndex: 1,
			},
			want: &Config{
				Port: "Root",
			},
		},
		{
			name: "Test #3",
			args: args{
				keyIndex:   1,
				BlockIndex: 0,
			},
			want: &Config{
				Port: "Root",
			},
		},
		{
			name: "Test #4",
			args: args{
				keyIndex:   1,
				BlockIndex: 1,
			},
			want: &Config{
				Port: "Root",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tt.name)
			c.ServerBlockKeyIndex = tt.args.keyIndex
			c.ServerBlockIndex = tt.args.BlockIndex
			if got := GetConfig(c); got.Port == tt.want.Port {
				t.Errorf("GetConfig() = \n %+v, \n %+v", got, tt.want)
			}
		})
	}
}
