package dnstapio

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/reuseport"

	tap "github.com/dnstap/golang-dnstap"
	fs "github.com/farsightsec/golang-framestream"
)

func newMsg() tap.Dnstap {
	msgType := tap.Dnstap_MESSAGE
	return tap.Dnstap{Type: &msgType}
}

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
	transport := [2][]string{
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

		dio := New(param[0], l.Addr().String())
		if err := dio.connect(); err != nil {
			log.Fatal(err)
		}

		dio.Dnstap(newMsg())
		dio.enc.Flush()

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

	dio := New("tcp", l.Addr().String())
	dio.connect()
	defer dio.close()

	wg.Add(count)
	for i := 0; i < count; i++ {
		go func() {
			time.Sleep(50 * time.Millisecond)
			dio.Dnstap(newMsg())
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
	dio := New("tcp", addr)
	dio.connect()
	defer dio.close()

	dio.Dnstap(newMsg())

	wg.Wait()
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
		time.Sleep(time.Second)
		dio.Dnstap(newMsg())
	}

	wg.Wait()
}
