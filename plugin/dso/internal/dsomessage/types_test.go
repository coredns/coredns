package dsomessage

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/miekg/dns"
)

var (
	testMsg = &Msg{
		MsgHeader: MsgHeader{42, true, dns.RcodeStatefulTypeNotImplemented},
		TLV: []TLV{
			&KeepAlive{InactivityTimeoutDefault, KeepAliveIntervalDefault},
			&RetryDelay{60 * 1000},
			&EncryptionPadding{8},
			&Subscribe{"test.", dns.TypeA, dns.ClassINET},
			&Push{[]dns.RR{aRRf("test. IN A 192.0.2.1")}},
			&Unsubscribe{9000},
			&Reconfirm{aRRf("test. IN A 192.0.2.1")},
		},
	}
	testMsgBytes = []byte{
		0x00, 0x2a, 0xb0, 0x0b, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Header
		0x00, 0x01, 0x00, 0x08, // KeepAlive Header
		0x00, 0x00, 0x3a, 0x98, 0x00, 0x00, 0x3a, 0x98, // KeepAlive
		0x00, 0x02, 0x00, 0x04, // RetryDelay Header
		0x00, 0x00, 0xea, 0x60, // RetryDelay
		0x00, 0x03, 0x00, 0x08, // EncryptionPadding Header
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // EncryptionPadding
		0x00, 0x40, 0x00, 0x0a, // Subscribe Header
		0x04, 0x74, 0x65, 0x73, 0x74, 0x00, 0x00, 0x01, 0x00, 0x01, // Subscribe
		0x00, 0x41, 0x00, 0x14, // Push Header
		0x04, 0x74, 0x65, 0x73, 0x74, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x0e, 0x10, 0x00, 0x04, 0xc0, 0x00, 0x02, 0x01, // Push
		0x00, 0x42, 0x00, 0x02, // Unsubscribe Header
		0x23, 0x28, // Unsubscribe
		0x00, 0x43, 0x00, 0x0e, // Reconfirm Header
		0x04, 0x74, 0x65, 0x73, 0x74, 0x00, 0x00, 0x01, 0x00, 0x01, 0xc0, 0x00, 0x02, 0x01, // Reconfirm
	}
	testMsgCompressedBytes = []byte{
		0x00, 0x2a, 0xb0, 0x0b, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Header
		0x00, 0x01, 0x00, 0x08, // KeepAlive Header
		0x00, 0x00, 0x3a, 0x98, 0x00, 0x00, 0x3a, 0x98, // KeepAlive
		0x00, 0x02, 0x00, 0x04, // RetryDelay Header
		0x00, 0x00, 0xea, 0x60, // RetryDelay
		0x00, 0x03, 0x00, 0x08, // EncryptionPadding Header
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // EncryptionPadding
		0x00, 0x40, 0x00, 0x0a, // Subscribe Header
		0x04, 0x74, 0x65, 0x73, 0x74, 0x00, 0x00, 0x01, 0x00, 0x01, // Subscribe
		0x00, 0x41, 0x00, 0x10, // Push Header
		0xc0, 0x30, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x0e, 0x10, 0x00, 0x04, 0xc0, 0x00, 0x02, 0x01, // Push
		0x00, 0x42, 0x00, 0x02, // Unsubscribe Header
		0x23, 0x28, // Unsubscribe
		0x00, 0x43, 0x00, 0x0e, // Reconfirm Header
		0x4, 0x74, 0x65, 0x73, 0x74, 0x0, 0x0, 0x1, 0x0, 0x1, 0xc0, 0x0, 0x2, 0x1, // Reconfirm
	}
)

func aRRf(format string, a ...any) *dns.A {
	r, _ := dns.NewRR(fmt.Sprintf(format, a...))
	return r.(*dns.A)
}

func txtRRf(format string, a ...any) *dns.TXT {
	r, _ := dns.NewRR(fmt.Sprintf(format, a...))
	return r.(*dns.TXT)
}

func TestBuilderParserParity(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name     string
		compress bool
		msgBytes []byte
	}{
		{
			"uncompressed",
			false,
			testMsgBytes,
		},
		{
			"compressed",
			true,
			testMsgCompressedBytes,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := NewBuilder(make([]byte, 127))
			if tc.compress {
				b.EnableCompression()
			}
			testMsg := testMsg.Clone()
			testMsg.PackTo(b)
			if err := b.Err(); err != nil {
				t.Fatalf("Expected to pack message, got %v", err)
			}
			msgBytes, _ := b.Message()
			if !slices.Equal(msgBytes, tc.msgBytes) {
				t.Errorf("Expected packed messages to be equal")
			}

			msg, err := UnpackMsg(msgBytes, OriginClient)
			if err != nil {
				t.Fatalf("Expected to unpack message, got %v", err)
			}
			if !msg.Equal(testMsg) {
				t.Errorf("Expected messages to be equal")
			}
		})
	}
}

func TestHeader(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name             string
		header           MsgHeader
		isRequest        bool
		isUnidirectional bool
		isResponse       bool
		err              error
	}{
		{
			"request",
			MsgHeader{1, false, 0},
			true,
			false,
			false,
			nil,
		},
		{
			"unidirectional",
			MsgHeader{0, false, 0},
			false,
			true,
			false,
			nil,
		},
		{
			"response",
			MsgHeader{1, true, 0},
			false,
			false,
			true,
			nil,
		},
		{
			"invalid",
			MsgHeader{0, true, 0},
			false,
			false,
			false,
			ErrHeader,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.header.IsRequest() != tc.isRequest {
				t.Errorf("Expected IsRequest() to be %v, got %v", tc.isRequest, !tc.isRequest)
			}
			if tc.header.IsUnidirectional() != tc.isUnidirectional {
				t.Errorf("Expected IsUnidirectional() to be %v, got %v", tc.isUnidirectional, !tc.isUnidirectional)
			}
			if tc.header.IsResponse() != tc.isResponse {
				t.Errorf("Expected IsResponse() to be %v, got %v", tc.isResponse, !tc.isResponse)
			}
			if err := tc.header.Verify(); !errors.Is(err, tc.err) {
				t.Errorf("Expected verify to return %v, got %v", tc.err, err)
			}
		})
	}
}
