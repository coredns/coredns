package dnsserver

import (
	"net"
	"net/http"
	"reflect"
	"testing"
)

func TestDoHWriter_LocalAddr(t *testing.T) {
	tests := []struct {
		name  string
		laddr net.Addr
		want  net.Addr
	}{
		{
			name:  "LocalAddr",
			laddr: &net.TCPAddr{},
			want:  &net.TCPAddr{},
		},
		{
			name:  "LocalAddr",
			laddr: &net.UDPAddr{},
			want:  &net.UDPAddr{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DoHWriter{
				laddr: tt.laddr,
			}
			if got := d.LocalAddr(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LocalAddr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDoHWriter_RemoteAddr(t *testing.T) {
	tests := []struct {
		name  string
		want  net.Addr
		raddr net.Addr
	}{
		{
			name:  "RemoteAddr",
			want:  &net.TCPAddr{},
			raddr: &net.TCPAddr{},
		},
		{
			name:  "RemoteAddr",
			want:  &net.UDPAddr{},
			raddr: &net.UDPAddr{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DoHWriter{
				raddr: tt.raddr,
			}
			if got := d.RemoteAddr(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RemoteAddr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDoHWriter_Request(t *testing.T) {
	tests := []struct {
		name    string
		request *http.Request
		want    *http.Request
	}{
		{
			name:    "Request",
			request: &http.Request{},
			want:    &http.Request{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DoHWriter{
				request: tt.request,
			}
			if got := d.Request(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Request() = %v, want %v", got, tt.want)
			}
		})
	}
}
