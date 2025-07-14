package dnsserver

import (
	"net"
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

func TestDoQWriter_ResponseWriterMethods(t *testing.T) {
	localAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
	remoteAddr := &net.UDPAddr{IP: net.ParseIP("8.8.8.8"), Port: 53}

	writer := &DoQWriter{
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}

	if err := writer.TsigStatus(); err != nil {
		t.Errorf("TsigStatus() returned an error: %v", err)
	}

	// this is a no-op, just call it
	writer.TsigTimersOnly(true)
	writer.TsigTimersOnly(false)

	// this is a no-op, just call it
	writer.Hijack()

	if addr := writer.LocalAddr(); addr != localAddr {
		t.Errorf("LocalAddr() = %v, want %v", addr, localAddr)
	}

	if addr := writer.RemoteAddr(); addr != remoteAddr {
		t.Errorf("RemoteAddr() = %v, want %v", addr, remoteAddr)
	}
}
