package dnstapio

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/golang/protobuf/proto"

	tap "github.com/dnstap/golang-dnstap"
	fs "github.com/farsightsec/golang-framestream"
)

var log = clog.NewWithPlugin("dnstap")

const (
	tcpTimeout   = 4 * time.Second
	flushTimeout = 1 * time.Second
)

// Tapper interface is used in testing to mock the Dnstap method.
type Tapper interface {
	Dnstap(tap.Dnstap)
}

// dio implements the Tapper interface.
type dio struct {
	endpoint string
	proto    string
	conn     net.Conn
	enc      *fs.Encoder
	dropped  uint32
	quit     chan struct{}

	sync.Mutex
}

// New returns a new and initialized pointer to a dio.
func New(proto, endpoint string) *dio {
	return &dio{
		proto:    proto,
		endpoint: endpoint,
		quit:     make(chan struct{}),
	}
}

// Connect connects to the socket.
func (d *dio) Connect() (err error) {
	d.conn, err = net.Dial(d.proto, d.endpoint)
	if err != nil {
		return err
	}
	d.enc, err = fs.NewEncoder(d.conn, &fs.EncoderOptions{
		ContentType: []byte("protobuf:dnstap.Dnstap"),
	})
	return err
}

// Serve connects to the dnstap endpoint and starts a maintenance go routine.
func (d *dio) Serve() {
	if err := d.Connect(); err != nil {
		log.Errorf("No connection to dnstap endpoint: %s", err)
	}
	go d.serve()
}

// Dnstap enqueues the payload for log.
func (d *dio) Dnstap(payload tap.Dnstap) {
	buf, err := proto.Marshal(&payload)
	if err != nil {
		atomic.AddUint32(&d.dropped, 1)
		return
	}
	d.Lock()
	if d.enc == nil {
		atomic.AddUint32(&d.dropped, 1)
		d.Unlock()
		return
	}
	_, err = d.enc.Write(buf)
	d.Unlock()
	if err != nil {
		atomic.AddUint32(&d.dropped, 1)
	}
	return
}

func (d *dio) flush() error {
	d.Lock()
	if d.enc != nil {
		return fmt.Errorf("no connection")
	}
	err := d.enc.Flush()
	d.Unlock()
	return err
}

func (d *dio) Close() {
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
}

func (d *dio) serve() {
	timeout := time.After(flushTimeout)
	for {
		select {
		case <-d.quit:
			d.Close()
			return
		case <-timeout:
			if dropped := atomic.SwapUint32(&d.dropped, 0); dropped > 0 {
				log.Warningf("Dropped dnstap messages: %d", dropped)
			}
			if err := d.flush(); err != nil {
				log.Errorf("Failed to flush dnstap: %s", err)
				d.Close()
			}
			// reconnect, if we've lost the connection
			if d.conn == nil {
				if err := d.Connect(); err != nil {
					log.Errorf("Cannot connect to dnstap: %s", err)
				} else {
					log.Info("Reconnected to dnstap")
				}
			}
			timeout = time.After(flushTimeout)
		}
	}
}
