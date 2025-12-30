package dnstap

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/reuseport"

	tap "github.com/dnstap/golang-dnstap"
	fs "github.com/farsightsec/golang-framestream"
	"github.com/stretchr/testify/require"
)

var (
	msgType = tap.Dnstap_MESSAGE
	tmsg    = tap.Dnstap{Type: &msgType}
)

type MockLogger struct {
	WarnCount int
	WarnLog   string
}

func (l *MockLogger) Warningf(format string, v ...any) {
	l.WarnCount++
	l.WarnLog += fmt.Sprintf(format, v...)
}

func accept(t *testing.T, l net.Listener, count int) {
	t.Helper()
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

	for range count {
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

		dio := newIO(param[0], l.Addr().String(), 1, 1)
		dio.tcpTimeout = 10 * time.Millisecond
		dio.flushTimeout = 30 * time.Millisecond
		dio.errorCheckInterval = 50 * time.Millisecond
		dio.connect()

		dio.Dnstap(&tmsg)

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

	dio := newIO("tcp", l.Addr().String(), 1, 1)
	dio.tcpTimeout = 10 * time.Millisecond
	dio.flushTimeout = 30 * time.Millisecond
	dio.errorCheckInterval = 50 * time.Millisecond
	dio.connect()
	defer dio.close()

	wg.Add(count)
	for range count {
		go func() {
			tmsg := tap.Dnstap_MESSAGE
			dio.Dnstap(&tap.Dnstap{Type: &tmsg})
			wg.Done()
		}()
	}
	wg.Wait()
}

func TestReconnect(t *testing.T) {
	t.Run("ConnectedOnStart", func(t *testing.T) {
		// GIVEN
		// 		TCP connection available before DnsTap start up
		//		DnsTap successfully established output connection on start up
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
		logger := MockLogger{}
		dio := newIO("tcp", addr, 1, 1)
		dio.tcpTimeout = 10 * time.Millisecond
		dio.flushTimeout = 30 * time.Millisecond
		dio.errorCheckInterval = 50 * time.Millisecond
		dio.logger = &logger
		dio.connect()
		defer dio.close()

		// WHEN
		//		TCP connection closed when DnsTap is still running
		//		TCP listener starts again on the same port
		//		DnsTap send multiple messages
		dio.Dnstap(&tmsg)
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

		messageCount := 5
		for range messageCount {
			time.Sleep(100 * time.Millisecond)
			dio.Dnstap(&tmsg)
		}
		wg.Wait()

		// THEN
		//		DnsTap is able to reconnect
		//		Messages can be sent eventually
		require.NotNil(t, dio.enc)
		require.Equal(t, 0, len(dio.queue))
		require.Less(t, logger.WarnCount, messageCount)
	})

	t.Run("NotConnectedOnStart", func(t *testing.T) {
		// GIVEN
		// 		No TCP connection established at DnsTap start up
		l, err := reuseport.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("Cannot start listener: %s", err)
		}
		l.Close()

		logger := MockLogger{}
		addr := l.Addr().String()
		dio := newIO("tcp", addr, 1, 1)
		dio.tcpTimeout = 10 * time.Millisecond
		dio.flushTimeout = 30 * time.Millisecond
		dio.errorCheckInterval = 50 * time.Millisecond
		dio.logger = &logger
		dio.connect()
		defer dio.close()

		// WHEN
		//		DnsTap is already running
		//		TCP listener starts on DnsTap's configured port
		//		DnsTap send multiple messages
		dio.Dnstap(&tmsg)

		l, err = reuseport.Listen("tcp", addr)
		if err != nil {
			t.Fatalf("Cannot start listener: %s", err)
		}
		defer l.Close()

		var wg sync.WaitGroup
		wg.Add(1)
		messageCount := 5
		go func() {
			accept(t, l, messageCount)
			wg.Done()
		}()

		for range messageCount {
			time.Sleep(100 * time.Millisecond)
			dio.Dnstap(&tmsg)
		}
		wg.Wait()

		// THEN
		//		DnsTap is able to reconnect
		//		Messages can be sent eventually
		require.NotNil(t, dio.enc)
		require.Equal(t, 0, len(dio.queue))
		require.Less(t, logger.WarnCount, messageCount)
	})
}

func TestFullQueueWriteFail(t *testing.T) {
	// GIVEN
	// 		DnsTap I/O with a small queue
	l, err := reuseport.Listen("unix", "dn2stap.sock")
	if err != nil {
		t.Fatalf("Cannot start listener: %s", err)
	}
	defer l.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		accept(t, l, 1)
		wg.Done()
	}()

	logger := MockLogger{}
	dio := newIO("unix", l.Addr().String(), 1, 1)
	dio.flushTimeout = 500 * time.Millisecond
	dio.errorCheckInterval = 50 * time.Millisecond
	dio.logger = &logger
	dio.queue = make(chan *tap.Dnstap, 1)
	dio.connect()
	defer dio.close()

	// WHEN
	//		messages overwhelms the queue
	count := 100
	for range count {
		dio.Dnstap(&tmsg)
	}
	wg.Wait()

	// THEN
	//		Dropped messages are logged
	require.NotEqual(t, 0, logger.WarnCount)
	require.Contains(t, logger.WarnLog, "Dropped dnstap messages")
}
