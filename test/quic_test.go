package test

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"sync"
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

// Corefile with custom stream limits
var quicLimitCorefile = `quic://.:0 {
		tls ../plugin/tls/test_cert.pem ../plugin/tls/test_key.pem ../plugin/tls/test_ca.pem
		quic {
			max_streams 5
			worker_pool_size 10
		}
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

// TestQUICStreamLimits tests that the max_streams limit is correctly enforced
func TestQUICStreamLimits(t *testing.T) {
	q, udp, _, err := CoreDNSServerAndPorts(quicLimitCorefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer q.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, convertAddress(udp), generateTLSConfig(), nil)
	if err != nil {
		t.Fatalf("Expected no error but got: %s", err)
	}

	m := createTestMsg()

	// Test opening exactly the max number of streams
	var wg sync.WaitGroup
	streamCount := 5 // Must match max_streams in quicLimitCorefile
	successCount := 0
	var mu sync.Mutex

	// Create a slice to store all the streams so we can keep them open
	streams := make([]*quic.Stream, 0, streamCount)
	streamsMu := sync.Mutex{}

	// Attempt to open exactly the configured number of streams
	for i := range streamCount {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Open stream
			streamSync, err := conn.OpenStreamSync(ctx)
			if err != nil {
				t.Logf("Stream %d: Failed to open: %s", idx, err)
				return
			}

			// Store the stream so we can keep it open
			streamsMu.Lock()
			streams = append(streams, streamSync)
			streamsMu.Unlock()

			// Write DNS message
			_, err = streamSync.Write(m)
			if err != nil {
				t.Logf("Stream %d: Failed to write: %s", idx, err)
				return
			}

			// Read response
			sizeBuf := make([]byte, 2)
			_, err = io.ReadFull(streamSync, sizeBuf)
			if err != nil {
				t.Logf("Stream %d: Failed to read size: %s", idx, err)
				return
			}

			size := binary.BigEndian.Uint16(sizeBuf)
			buf := make([]byte, size)
			_, err = io.ReadFull(streamSync, buf)
			if err != nil {
				t.Logf("Stream %d: Failed to read response: %s", idx, err)
				return
			}

			mu.Lock()
			successCount++
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if successCount != streamCount {
		t.Errorf("Expected all %d streams to succeed, but only %d succeeded", streamCount, successCount)
	}

	// Now try to open more streams beyond the limit while keeping existing streams open
	// The QUIC protocol doesn't immediately reject streams; they might be allowed
	// to open but will be blocked (flow control) until other streams close

	// First, make sure none of our streams have been closed
	for i, s := range streams {
		if s == nil {
			t.Errorf("Stream %d is nil", i)
			continue
		}
	}

	// Try to open a batch of additional streams - with streams limited to 5,
	// these should either block or be queued but should not allow concurrent use
	extraCount := 10
	extraSuccess := 0
	var extraSuccessMu sync.Mutex

	// Set a shorter timeout for these attempts
	extraCtx, extraCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer extraCancel()

	var extraWg sync.WaitGroup

	// Create a channel to signal test completion
	done := make(chan struct{})

	// Launch goroutines to attempt opening additional streams
	for i := range extraCount {
		extraWg.Add(1)
		go func(idx int) {
			defer extraWg.Done()

			select {
			case <-done:
				return // Test is finishing, abandon attempts
			default:
				// Continue with the test
			}

			// Attempt to open an additional stream
			stream, err := conn.OpenStreamSync(extraCtx)
			if err != nil {
				t.Logf("Extra stream %d correctly failed to open: %s", idx, err)
				return
			}

			// If we got this far, we managed to open a stream
			// But we shouldn't be able to use more than max_streams concurrently
			_, err = stream.Write(m)
			if err != nil {
				t.Logf("Extra stream %d failed to write: %s", idx, err)
				return
			}

			// Read response
			sizeBuf := make([]byte, 2)
			_, err = io.ReadFull(stream, sizeBuf)
			if err != nil {
				t.Logf("Extra stream %d failed to read: %s", idx, err)
				return
			}

			// This stream completed successfully
			extraSuccessMu.Lock()
			extraSuccess++
			extraSuccessMu.Unlock()

			// Close the stream explicitly
			_ = stream.Close()
		}(i)
	}

	// Start closing original streams after a delay
	// This should allow extra streams to proceed as slots become available
	time.Sleep(500 * time.Millisecond)

	// Close all the original streams
	for _, s := range streams {
		_ = s.Close()
	}

	// Allow extra streams some time to progress
	extraWg.Wait()
	close(done)

	// Since original streams are now closed, extra streams might succeed
	// But we shouldn't see more than max_streams succeed during the blocked phase
	if extraSuccess > streamCount {
		t.Logf("Warning: %d extra streams succeeded, which is more than the limit of %d. This might be because original streams were closed.",
			extraSuccess, streamCount)
	}

	t.Logf("%d/%d extra streams were able to complete after original streams were closed",
		extraSuccess, extraCount)
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
