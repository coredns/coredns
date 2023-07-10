package dnsserver

import (
	"testing"
)

func TestDoQWriterAddPrefix(t *testing.T) {
	byteArray := []byte{0x1, 0x2, 0x3}

	byteArrayWithPrefix := AddPrefix(byteArray)

	if len(byteArrayWithPrefix) != 5 {
		t.Error("Expected byte array with prefix to have length of 5")
	}

	size := int16(byteArrayWithPrefix[0])<<8 | int16(byteArrayWithPrefix[1])
	if size != 3 {
		t.Errorf("Expected prefixed size to be 3, got: %d", size)
	}
}
