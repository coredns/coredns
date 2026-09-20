package forward

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

func TestForwardDoQIntegration(t *testing.T) {
	serverTLS, roots := makeForwardDoQTestTLS(t)
	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, &quic.Config{MaxIncomingStreams: 16})
	if err != nil {
		t.Fatalf("quic.ListenAddr() failed: %v", err)
	}
	defer listener.Close()

	serverResult := make(chan error, 1)
	go func() {
		conn, err := listener.Accept(context.Background())
		if err != nil {
			serverResult <- err
			return
		}
		if got := conn.ConnectionState().TLS.NegotiatedProtocol; got != "doq" {
			serverResult <- fmt.Errorf("negotiated ALPN = %q, want doq", got)
			return
		}
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			serverResult <- err
			return
		}
		_ = stream.SetDeadline(time.Now().Add(2 * time.Second))
		query, err := readForwardDoQMessage(stream)
		if err != nil {
			serverResult <- err
			return
		}
		var extra [1]byte
		if n, err := stream.Read(extra[:]); n != 0 || !errors.Is(err, io.EOF) {
			serverResult <- fmt.Errorf("query stream did not end with FIN: n=%d err=%v", n, err)
			return
		}
		if query.Id != 0 {
			serverResult <- fmt.Errorf("query ID = %d, want 0", query.Id)
			return
		}

		response := new(dns.Msg)
		response.SetReply(query)
		record, err := dns.NewRR("example.org. 60 IN A 192.0.2.53")
		if err != nil {
			serverResult <- err
			return
		}
		response.Answer = []dns.RR{record}
		wire, err := response.Pack()
		if err != nil {
			serverResult <- err
			return
		}
		frame := make([]byte, 2+len(wire))
		binary.BigEndian.PutUint16(frame, uint16(len(wire))) // #nosec G115 -- DNS wire size is bounded by Pack
		copy(frame[2:], wire)
		for len(frame) > 0 {
			n, err := stream.Write(frame)
			if err != nil {
				serverResult <- err
				return
			}
			if n == 0 {
				serverResult <- io.ErrShortWrite
				return
			}
			frame = frame[n:]
		}
		if err := stream.Close(); err != nil {
			serverResult <- err
			return
		}
		serverResult <- nil
	}()

	c := caddy.NewTestController("dns", fmt.Sprintf(`forward . quic://%s {
		tls_servername doq.test
	}`, listener.Addr()))
	fs, err := parseForward(c)
	if err != nil {
		t.Fatalf("parseForward() failed: %v", err)
	}
	f := fs[0]
	clientTLS := f.proxies[0].GetTransport().GetTLSConfig().Clone()
	clientTLS.RootCAs = roots
	f.proxies[0].SetTLSConfig(clientTLS)
	if err := f.OnStartup(); err != nil {
		t.Fatalf("OnStartup() failed: %v", err)
	}
	defer f.OnShutdown()

	query := new(dns.Msg)
	query.SetQuestion("example.org.", dns.TypeA)
	query.Id = 0x4321
	recorder := dnstest.NewRecorder(&test.ResponseWriter{})
	if _, err := f.ServeDNS(context.Background(), recorder, query); err != nil {
		t.Fatalf("ServeDNS() failed: %v", err)
	}
	if recorder.Msg == nil || recorder.Msg.Id != 0x4321 {
		t.Fatalf("response ID = %v, want %d", recorder.Msg, 0x4321)
	}
	if len(recorder.Msg.Answer) != 1 || recorder.Msg.Answer[0].String() != "example.org.\t60\tIN\tA\t192.0.2.53" {
		t.Fatalf("unexpected response answers: %v", recorder.Msg.Answer)
	}
	select {
	case err := <-serverResult:
		if err != nil {
			t.Fatalf("DoQ upstream failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("DoQ upstream did not finish")
	}

	transfer := new(dns.Msg)
	transfer.SetQuestion("example.org.", dns.TypeAXFR)
	rcode, err := f.ServeDNS(context.Background(), &test.ResponseWriter{}, transfer)
	if rcode != dns.RcodeNotImplemented || err == nil {
		t.Fatalf("AXFR over DoQ returned rcode=%d err=%v, want NOTIMP with an error", rcode, err)
	}
}

func readForwardDoQMessage(r io.Reader) (*dns.Msg, error) {
	var size [2]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return nil, err
	}
	wire := make([]byte, int(binary.BigEndian.Uint16(size[:])))
	if len(wire) == 0 {
		return nil, errors.New("zero-length DoQ message")
	}
	if _, err := io.ReadFull(r, wire); err != nil {
		return nil, err
	}
	msg := new(dns.Msg)
	if err := msg.Unpack(wire); err != nil {
		return nil, err
	}
	return msg, nil
}

func makeForwardDoQTestTLS(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey() failed: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "doq.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"doq.test"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() failed: %v", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate() failed: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: privateKey}},
		NextProtos:   []string{"doq"},
	}, roots
}
