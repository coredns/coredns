package dnstapio

import (
	"net"
	"sync/atomic"
	"time"

	clog "github.com/coredns/coredns/plugin/pkg/log"

	tap "github.com/dnstap/golang-dnstap"
	fs "github.com/farsightsec/golang-framestream"
)

var log = clog.NewWithPlugin("dnstap")

const (
	tcpWriteBufSize = 1024 * 1024
	tcpTimeout      = 4 * time.Second
	flushTimeout    = 1 * time.Second
	queueSize       = 10000
)

// Tapper interface is used in testing to mock the Dnstap method.
type Tapper interface {
	Dnstap(tap.Dnstap)
}

// dio implements the Tapper interface.
type dio struct {
	endpoint string
	socket   bool
	conn     net.Conn
	enc      *dnstapEncoder
	queue    chan tap.Dnstap
	dropped  uint32
	quit     chan struct{}
}

// New returns a new and initialized pointer to a dio.
func New(endpoint string, socket bool) *dio {
	return &dio{
		endpoint: endpoint,
		socket:   socket,
		enc: newDnstapEncoder(&fs.EncoderOptions{
			ContentType:   []byte("protobuf:dnstap.Dnstap"),
			Bidirectional: true,
		}),
		queue: make(chan tap.Dnstap, queueSize),
		quit:  make(chan struct{}),
	}
}

func (d *dio) newConnect() error {
	var err error
	if d.socket {
		if d.conn, err = net.Dial("unix", d.endpoint); err != nil {
			return err
		}
	} else {
		if d.conn, err = net.DialTimeout("tcp", d.endpoint, tcpTimeout); err != nil {
			return err
		}
		if tcpConn, ok := d.conn.(*net.TCPConn); ok {
			tcpConn.SetWriteBuffer(tcpWriteBufSize)
			tcpConn.SetNoDelay(false)
		}
	}

	return d.enc.resetWriter(d.conn)
}

// Connect connects to the dnstap endpoint.
func (d *dio) Connect() {
	if err := d.newConnect(); err != nil {
		log.Error("No connection to dnstap endpoint")
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

func (d *dio) closeConnection() {
	d.enc.close()
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
}

// Close waits until the I/O routine is finished to return.
func (d *dio) Close() { close(d.quit) }

func (d *dio) flushBuffer() {
	if d.conn == nil {
		if err := d.newConnect(); err != nil {
			return
		}
		log.Info("Reconnected to dnstap")
	}

	if err := d.enc.flushBuffer(); err != nil {
		log.Warningf("Connection lost: %s", err)
		d.closeConnection()
		if err := d.newConnect(); err != nil {
			log.Errorf("Cannot connect to dnstap: %s", err)
		} else {
			log.Info("Reconnected to dnstap")
		}
	}
}

func (d *dio) write(payload *tap.Dnstap) {
	if err := d.enc.writeMsg(payload); err != nil {
		atomic.AddUint32(&d.dropped, 1)
	}
}

func (d *dio) serve() {
	timeout := time.After(flushTimeout)
	for {
		select {
		case <-d.quit:
			d.flushBuffer()
			d.closeConnection()
			return
		case payload := <-d.queue:
			d.write(&payload)
		case <-timeout:
			if dropped := atomic.SwapUint32(&d.dropped, 0); dropped > 0 {
				log.Warningf("Dropped dnstap messages: %d", dropped)
			}
			d.flushBuffer()
			timeout = time.After(flushTimeout)
		}
	}
}
