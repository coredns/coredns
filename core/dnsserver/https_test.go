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

func TestDoHWriter_Write(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{
			name: "valid DNS message",
			// A minimal valid DNS query message
			input: []byte{
				0x00, 0x01, /* ID */
				0x01, 0x00, /* Flags: query, recursion desired */
				0x00, 0x01, /* Questions: 1 */
				0x00, 0x00, /* Answer RRs: 0 */
				0x00, 0x00, /* Authority RRs: 0 */
				0x00, 0x00, /* Additional RRs: 0 */
				0x03, 'w', 'w', 'w',
				0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
				0x03, 'c', 'o', 'm',
				0x00,       /* Null terminator for domain name */
				0x00, 0x01, /* Type: A */
				0x00, 0x01, /* Class: IN */
			},
			wantErr: false,
		},
		{
			name:    "empty message",
			input:   []byte{},
			wantErr: true, // Expect an error because unpacking an empty message will fail
		},
		{
			name:    "invalid DNS message",
			input:   []byte{0x00, 0x01, 0x02}, // Truncated message
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DoHWriter{}
			n, err := d.Write(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && n != len(tt.input) {
				t.Errorf("Write() bytes written = %v, want %v", n, len(tt.input))
			}
			if !tt.wantErr && d.Msg == nil {
				t.Errorf("Write() d.Msg is nil, expected a parsed message")
			}
		})
	}
}

func TestDoHWriter_Close(t *testing.T) {
	d := &DoHWriter{}
	if err := d.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestDoHWriter_TsigStatus(t *testing.T) {
	d := &DoHWriter{}
	if err := d.TsigStatus(); err != nil {
		t.Errorf("TsigStatus() error = %v, want nil", err)
	}
}
