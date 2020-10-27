package dnstapio

import (
	"net"
	"sync/atomic"
	"time"

	clog "github.com/coredns/coredns/plugin/pkg/log"

	tap "github.com/dnstap/golang-dnstap"
)

var log = clog.NewWithPlugin("dnstap")

const (
	tcpWriteBufSize = 1024 * 1024 // there is no good explanation for why this number (see #xxx)
	queueSize       = 10000       // see #xxxx
	tcpTimeout      = 4 * time.Second
	flushTimeout    = 1 * time.Second
)

// Tapper interface is used in testing to mock the Dnstap method.
type Tapper interface {
	Dnstap(tap.Dnstap)
}

// dio implements the Tapper interface.
type dio struct {
	endpoint     string
	proto        string
	conn         net.Conn
	enc          *Encoder
	queue        chan tap.Dnstap
	dropped      uint32
	quit         chan struct{}
	flushTimeout time.Duration
	tcpTimeout   time.Duration
}

// New returns a new and initialized pointer to a dio.
func New(proto, endpoint string) *dio {
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
func (d *dio) Connect() {
	if err := d.dial(); err != nil {
		log.Errorf("No connection to dnstap endpoint: %s", err)
	}
	go d.serve()
}

// Dnstap enqueues the payload for log.
func (d *dio) Dnstap(payload tap.Dnstap) {
	select {
	case d.queue <- payload:
	default:
		atomic.AddUint32(&d.dropped, 1)
	}
}

// Close waits until the I/O routine is finished to return.
func (d *dio) Close() { close(d.quit) }

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
