package dsomessage

import (
	"reflect"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func unmarshalTLV(tb testing.TB, tp Type, data []byte) (tlv TLV, err error) {
	tb.Helper()

	switch tp {
	case TypeKeepAlive:
		tlv = &KeepAlive{}
	case TypeRetryDelay:
		tlv = &RetryDelay{}
	case TypeEncryptionPadding:
		tlv = &EncryptionPadding{}
	case TypeSubscribe:
		tlv = &Subscribe{}
	case TypePush:
		tlv = &Push{}
	case TypeUnsubscribe:
		tlv = &Unsubscribe{}
	case TypeReconfirm:
		tlv = &Reconfirm{}
	default:
		tb.Fatalf("Unexpected TLV of type %v", tp)
	}

	val := reflect.ValueOf(tlv)
	// ptr := reflect.New(val.Type())
	// ptr.Elem().Set(val)

	method := val.MethodByName("UnmarshalBinary")
	result := method.Call([]reflect.Value{reflect.ValueOf(data)})
	if !result[0].IsNil() {
		err = result[0].Interface().(error)
	}
	return val.Interface().(TLV), err
}

func TestEncodingTLVMarshaller(t *testing.T) {
	t.Parallel()

	var (
		testMsg  = testMsg.Clone()
		msgBytes []byte
	)

	t.Run("marshal", func(t *testing.T) {
		headerBytes, _ := testMsg.MsgHeader.MarshalBinary()
		msgBytes = append(msgBytes, headerBytes...)
		for _, tlv := range testMsg.TLV {
			tlvBytes, _ := tlv.MarshalBinary()
			tlvHeaderBytes, _ := TLVHeader{tlv.Type(), uint16(len(tlvBytes))}.MarshalBinary()
			msgBytes = append(msgBytes, tlvHeaderBytes...)
			msgBytes = append(msgBytes, tlvBytes...)
		}
		if !slices.Equal(msgBytes, testMsgBytes) {
			t.Error(cmp.Diff(testMsgBytes, msgBytes))
		}
	})

	t.Run("unmarshal", func(t *testing.T) {
		var (
			msg Msg
			off = MsgHeaderLen
		)
		msg.MsgHeader.UnmarshalBinary(msgBytes[:off])
		for off < len(msgBytes) {
			var tlvHeader TLVHeader
			err := tlvHeader.UnmarshalBinary(msgBytes[off : off+TLVHeaderLen])
			if err != nil {
				t.Fatalf("Got TLVHeader.UnmarshalBinary() = %v, want <nil>", err)
			}
			tlv, err := unmarshalTLV(t, tlvHeader.Type, msgBytes[off+TLVHeaderLen:off+TLVHeaderLen+int(tlvHeader.Length)])
			if err != nil {
				t.Fatalf("Got %s.UnmarshalBinary() = %v, want <nil>", tlvHeader.Type.String(), err)
			}
			msg.TLV = append(msg.TLV, tlv)
			off += TLVHeaderLen + int(tlvHeader.Length)
		}
		if !msg.Equal(testMsg) {
			t.Error(cmp.Diff(testMsg, msg))
		}
	})
}

func TestEncodingTLVAppender(t *testing.T) {
	t.Parallel()

	prefix := slices.Repeat([]byte{1}, 16)
	msgBytes := slices.Clone(prefix)
	testMsg := testMsg.Clone()

	msgBytes, _ = testMsg.MsgHeader.AppendBinary(msgBytes)
	for _, tlv := range testMsg.TLV {
		headerStart := len(msgBytes)
		msgBytes = append(msgBytes, make([]byte, TLVHeaderLen)...)
		headerEnd := len(msgBytes)
		msgBytes, _ = tlv.AppendBinary(msgBytes)
		TLVHeader{tlv.Type(), uint16(len(msgBytes) - headerEnd)}.AppendBinary(msgBytes[:headerStart])
	}
	if !slices.Equal(msgBytes[:len(prefix)], prefix) {
		t.Errorf("Want unchanged prefix")
	}
	if !slices.Equal(msgBytes[len(prefix):], testMsgBytes) {
		t.Error(cmp.Diff(testMsgBytes, msgBytes))
	}
}

func TestEncodingMsgMarshaller(t *testing.T) {
	t.Parallel()

	var (
		msgBytes []byte
		testMsg  = testMsg.Clone()
	)

	t.Run("marshal", func(t *testing.T) {
		msgBytes, _ = testMsg.MarshalBinary()
		if !slices.Equal(msgBytes, testMsgBytes) {
			t.Error(cmp.Diff(testMsgBytes, msgBytes))
		}
	})

	t.Run("unmarshal", func(t *testing.T) {
		msg := new(Msg)
		msg.UnmarshalBinary(msgBytes)
		if !msg.Equal(testMsg) {
			t.Error(cmp.Diff(testMsg, msg))
		}
	})
}

func TestEncodingMsgAppender(t *testing.T) {
	t.Parallel()

	prefix := slices.Repeat([]byte{1}, 16)
	msgBytes := slices.Clone(prefix)

	msgBytes, _ = testMsg.Clone().AppendBinary(msgBytes)
	if !slices.Equal(msgBytes[:len(prefix)], prefix) {
		t.Errorf("Want unchanged prefix")
	}
	if !slices.Equal(msgBytes[len(prefix):], testMsgBytes) {
		t.Error(cmp.Diff(testMsgBytes, msgBytes))
	}
}
