package test

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	ctls "github.com/coredns/coredns/plugin/pkg/tls"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

var quicCorefile = `quic://.:0 {
		tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem ../plugin/tls/test_ca.pem
		whoami
	}`

func TestQUIC(t *testing.T) {
	q, udp, _, err := CoreDNSServerAndPorts(quicCorefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer q.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, convertAddress(udp), generateTLSConfig(), nil)
	if err != nil {
		t.Fatalf("Expected no error but got: %s", err)
	}

	m := createTestMsg()

	streamSync, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}

	_, err = streamSync.Write(m)
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}
	_ = streamSync.Close()

	sizeBuf := make([]byte, 2)
	_, err = io.ReadFull(streamSync, sizeBuf)
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}

	size := binary.BigEndian.Uint16(sizeBuf)
	buf := make([]byte, size)
	_, err = io.ReadFull(streamSync, buf)
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}

	d := new(dns.Msg)
	err = d.Unpack(buf)
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}

	if d.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected success but got %d", d.Rcode)
	}

	if len(d.Extra) != 2 {
		t.Errorf("Expected 2 RRs in additional section, but got %d", len(d.Extra))
	}
}

func TestQUICProtocolError(t *testing.T) {
	q, udp, _, err := CoreDNSServerAndPorts(quicCorefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer q.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, convertAddress(udp), generateTLSConfig(), nil)
	if err != nil {
		t.Fatalf("Expected no error but got: %s", err)
	}

	m := createInvalidDOQMsg()

	streamSync, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}

	_, err = streamSync.Write(m)
	if err != nil {
		t.Errorf("Expected no error but got: %s", err)
	}
	_ = streamSync.Close()

	errorBuf := make([]byte, 2)
	_, err = io.ReadFull(streamSync, errorBuf)
	if err == nil {
		t.Errorf("Expected protocol error but got: %s", errorBuf)
	}

	if !isProtocolErr(err) {
		t.Errorf("Expected \"Application Error 0x2\" but got: %s", err)
	}
}

func isProtocolErr(err error) bool {
	var qAppErr *quic.ApplicationError
	return errors.As(err, &qAppErr) && qAppErr.ErrorCode == 2
}

// convertAddress transforms the address given in CoreDNSServerAndPorts to a format
// that quic.DialAddr can read. It is unable to use [::]:61799, see:
// "INTERNAL_ERROR (local): write udp [::]:50676->[::]:61799: sendmsg: no route to host"
// So it transforms it to localhost:61799.
func convertAddress(address string) string {
	if strings.HasPrefix(address, "[::]") {
		address = strings.Replace(address, "[::]", "localhost", 1)
	}
	return address
}

func generateTLSConfig() *tls.Config {
	tlsConfig, err := ctls.NewTLSConfig(
		"../plugin/tls/test_cert.pem",
		"../plugin/tls/test_key.pem",
		"../plugin/tls/test_ca.pem")

	if err != nil {
		panic(err)
	}

	tlsConfig.NextProtos = []string{"doq"}
	tlsConfig.InsecureSkipVerify = true

	return tlsConfig
}

func createTestMsg() []byte {
	m := new(dns.Msg)
	m.SetQuestion("whoami.example.org.", dns.TypeA)
	m.Id = 0
	msg, _ := m.Pack()
	return dnsserver.AddPrefix(msg)
}

func createInvalidDOQMsg() []byte {
	m := new(dns.Msg)
	m.SetQuestion("whoami.example.org.", dns.TypeA)
	msg, _ := m.Pack()
	return msg
}
