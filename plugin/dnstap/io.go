package dnstap

import (
	"net"
	"sync/atomic"
	"time"

	tap "github.com/dnstap/golang-dnstap"
)

const (
	tcpWriteBufSize = 1024 * 1024 // there is no good explanation for why this number has this value.
	queueSize       = 10000       // idem.

	tcpTimeout   = 4 * time.Second
	flushTimeout = 1 * time.Second
)

// tapper interface is used in testing to mock the Dnstap method.
type tapper interface {
	Dnstap(tap.Dnstap)
}

// dio implements the Tapper interface.
type dio struct {
	endpoint     string
	proto        string
	conn         net.Conn
	enc          *encoder
	queue        chan tap.Dnstap
	dropped      uint32
	quit         chan struct{}
	flushTimeout time.Duration
	tcpTimeout   time.Duration
}

// newIO returns a new and initialized pointer to a dio.
func newIO(proto, endpoint string) *dio {
	return &dio{
		endpoint:     endpoint,
		proto:        proto,
		queue:        make(chan tap.Dnstap, queueSize),
		quit:         make(chan struct{}),
		flushTimeout: flushTimeout,
		tcpTimeout:   tcpTimeout,
	}
}

func (d *dio) dial() error {
	conn, err := net.DialTimeout(d.proto, d.endpoint, d.tcpTimeout)
	if err != nil {
		return err
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetWriteBuffer(tcpWriteBufSize)
		tcpConn.SetNoDelay(false)
	}

	d.enc, err = newEncoder(conn, d.tcpTimeout)
	return err
}

// Connect connects to the dnstap endpoint.
func (d *dio) connect() error {
	err := d.dial()
	go d.serve()
	return err
}

// Dnstap enqueues the payload for log.
func (d *dio) Dnstap(payload tap.Dnstap) {
	select {
	case d.queue <- payload:
	default:
		atomic.AddUint32(&d.dropped, 1)
	}
}

// close waits until the I/O routine is finished to return.
func (d *dio) close() { close(d.quit) }

func (d *dio) write(payload *tap.Dnstap) error {
	if d.enc == nil {
		atomic.AddUint32(&d.dropped, 1)
		return nil
	}
	if err := d.enc.writeMsg(payload); err != nil {
		atomic.AddUint32(&d.dropped, 1)
		return err
	}
	return nil
}

func (d *dio) serve() {
	timeout := time.After(d.flushTimeout)
	for {
		select {
		case <-d.quit:
			if d.enc == nil {
				return
			}
			d.enc.flush()
			d.enc.close()
			return
		case payload := <-d.queue:
			if err := d.write(&payload); err != nil {
				d.dial()
			}
		case <-timeout:
			if dropped := atomic.SwapUint32(&d.dropped, 0); dropped > 0 {
				log.Warningf("Dropped dnstap messages: %d", dropped)
			}
			if d.enc == nil {
				d.dial()
			} else {
				d.enc.flush()
			}
			timeout = time.After(d.flushTimeout)
		}
	}
}
