package dnstap

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/reuseport"

	tap "github.com/dnstap/golang-dnstap"
	fs "github.com/farsightsec/golang-framestream"
)

var (
	msgType = tap.Dnstap_MESSAGE
	tmsg    = tap.Dnstap{Type: &msgType}
)

func accept(t *testing.T, l net.Listener, count int) {
	server, err := l.Accept()
	if err != nil {
		t.Fatalf("Server accepted: %s", err)
	}
	dec, err := fs.NewDecoder(server, &fs.DecoderOptions{
		ContentType:   []byte("protobuf:dnstap.Dnstap"),
		Bidirectional: true,
	})
	if err != nil {
		t.Fatalf("Server decoder: %s", err)
	}

	for i := 0; i < count; i++ {
		if _, err := dec.Decode(); err != nil {
			t.Errorf("Server decode: %s", err)
		}
	}

	if err := server.Close(); err != nil {
		t.Error(err)
	}
}

func TestTransport(t *testing.T) {
	transport := [2][2]string{
		{"tcp", ":0"},
		{"unix", "dnstap.sock"},
	}

	for _, param := range transport {
		l, err := reuseport.Listen(param[0], param[1])
		if err != nil {
			t.Fatalf("Cannot start listener: %s", err)
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			accept(t, l, 1)
			wg.Done()
		}()

		dio := newIO(param[0], l.Addr().String())
		dio.tcpTimeout = 10 * time.Millisecond
		dio.flushTimeout = 30 * time.Millisecond
		dio.connect()

		dio.Dnstap(tmsg)

		wg.Wait()
		l.Close()
		dio.close()
	}
}

func TestRace(t *testing.T) {
	count := 10

	l, err := reuseport.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Cannot start listener: %s", err)
	}
	defer l.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		accept(t, l, count)
		wg.Done()
	}()

	dio := newIO("tcp", l.Addr().String())
	dio.tcpTimeout = 10 * time.Millisecond
	dio.flushTimeout = 30 * time.Millisecond
	dio.connect()
	defer dio.close()

	wg.Add(count)
	for i := 0; i < count; i++ {
		go func() {
			tmsg := tap.Dnstap_MESSAGE
			dio.Dnstap(tap.Dnstap{Type: &tmsg})
			wg.Done()
		}()
	}
	wg.Wait()
}

func TestReconnect(t *testing.T) {
	count := 5

	l, err := reuseport.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Cannot start listener: %s", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		accept(t, l, 1)
		wg.Done()
	}()

	addr := l.Addr().String()
	dio := newIO("tcp", addr)
	dio.tcpTimeout = 10 * time.Millisecond
	dio.flushTimeout = 30 * time.Millisecond
	dio.connect()
	defer dio.close()

	dio.Dnstap(tmsg)

	wg.Wait()

	// Close listener
	l.Close()
	// And start TCP listener again on the same port
	l, err = reuseport.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("Cannot start listener: %s", err)
	}
	defer l.Close()

	wg.Add(1)
	go func() {
		accept(t, l, 1)
		wg.Done()
	}()

	for i := 0; i < count; i++ {
		time.Sleep(100 * time.Millisecond)
		dio.Dnstap(tmsg)
	}
	wg.Wait()
}
