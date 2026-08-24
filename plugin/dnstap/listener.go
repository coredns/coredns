package dnstap

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"sync"
	"time"

	tap "github.com/dnstap/golang-dnstap"
)

// listener manages incoming dnstap sink connections.
// Unlike outgoing connections (dio), incoming connections have no buffering -
// if a client is slow or disconnected, messages are dropped for that client only.
type listener struct {
	endpoint   string
	proto      string // "unix", "tcp", or "tls"
	ln         net.Listener
	clients    map[*client]struct{}
	clientsMu  sync.RWMutex
	quit       chan struct{}
	tcpTimeout time.Duration
	skipVerify bool
	certFile   string
	keyFile    string
	caFile     string
	logger     WarnLogger
}

// client represents a single connected dnstap sink.
type client struct {
	conn net.Conn
	enc  *encoder
	quit chan struct{}
}

// newListener creates a new listener for incoming dnstap connections.
func newListener(proto, endpoint string) *listener {
	return &listener{
		endpoint:   endpoint,
		proto:      proto,
		clients:    make(map[*client]struct{}),
		quit:       make(chan struct{}),
		tcpTimeout: tcpTimeout,
		skipVerify: skipVerify,
		logger:     log,
	}
}

// loadCAPool loads a CA certificate pool from a PEM file.
func loadCAPool(caFile string) (*x509.CertPool, error) {
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		return nil, err
	}
	return pool, nil
}

// listen starts accepting incoming connections.
func (l *listener) listen() error {
	var ln net.Listener
	var err error

	switch l.proto {
	case "tls":
		if l.certFile == "" || l.keyFile == "" {
			l.logger.Warningf("TLS listener requires cert and key files")
			return nil
		}

		cert, err := tls.LoadX509KeyPair(l.certFile, l.keyFile)
		if err != nil {
			return err
		}

		config := &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.NoClientCert,
			// #nosec G402 -- optional, user-configurable escape hatch for environments that cannot validate certs.
			InsecureSkipVerify: l.skipVerify,
		}

		// If CA file is provided, enable and require client certificate verification
		if l.caFile != "" {
			pool, err := loadCAPool(l.caFile)
			if err != nil {
				return err
			}
			config.ClientCAs = pool
			if l.skipVerify {
				config.ClientAuth = tls.VerifyClientCertIfGiven
			} else {
				config.ClientAuth = tls.RequireAndVerifyClientCert
			}
		}

		ln, err = tls.Listen("tcp", l.endpoint, config)
		if err != nil {
			return err
		}
	case "tcp":
		ln, err = net.Listen("tcp", l.endpoint)
		if err != nil {
			return err
		}
	default:
		// unix socket
		ln, err = net.Listen("unix", l.endpoint)
		if err != nil {
			return err
		}
	}

	l.ln = ln
	go l.acceptLoop()
	return nil
}

// acceptLoop accepts new incoming connections and spawns a goroutine for each.
func (l *listener) acceptLoop() {
	for {
		select {
		case <-l.quit:
			return
		default:
		}

		conn, err := l.ln.Accept()
		if err != nil {
			select {
			case <-l.quit:
				return
			default:
				l.logger.Warningf("Error accepting dnstap connection: %v", err)
				time.Sleep(100 * time.Millisecond) // brief pause on error
				continue
			}
		}

		// Set TCP parameters for TCP connections
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetNoDelay(false)
		}

		// Create encoder for this client
		enc, err := newEncoder(conn, l.tcpTimeout)
		if err != nil {
			l.logger.Warningf("Error creating encoder for dnstap client: %v", err)
			conn.Close()
			continue
		}

		c := &client{
			conn: conn,
			enc:  enc,
			quit: make(chan struct{}),
		}

		l.clientsMu.Lock()
		l.clients[c] = struct{}{}
		l.clientsMu.Unlock()
	}
}

// Dnstap broadcasts the payload to all connected clients.
// Unlike dio.Dnstap(), this does not buffer; if a write fails, we just drop
// the message for that client and close its connection.
func (l *listener) Dnstap(payload *tap.Dnstap) {
	l.clientsMu.RLock()
	defer l.clientsMu.RUnlock()

	for c := range l.clients {
		select {
		case <-c.quit:
			// Client already closed
			continue
		default:
			if err := c.enc.writeMsg(payload); err != nil {
				go l.removeClient(c)
			} else if err := c.enc.flush(); err != nil {
				go l.removeClient(c)
			}
		}
	}
}

// removeClient removes a client from the active set and closes its connection.
func (l *listener) removeClient(c *client) {
	l.clientsMu.Lock()
	defer l.clientsMu.Unlock()

	if _, exists := l.clients[c]; !exists {
		return // already removed
	}

	delete(l.clients, c)

	// Close quit channel to signal goroutines to stop (this also stops the flush ticker)
	select {
	case <-c.quit:
		// Already closed
	default:
		close(c.quit)
	}

	if c.enc != nil {
		c.enc.flush()
		c.enc.close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

// close stops accepting new connections and closes all active clients.
func (l *listener) close() {
	close(l.quit)

	if l.ln != nil {
		l.ln.Close()
	}

	l.clientsMu.Lock()
	defer l.clientsMu.Unlock()

	for c := range l.clients {
		delete(l.clients, c)

		// Close quit channel to signal goroutines to stop
		select {
		case <-c.quit:
			// Already closed
		default:
			close(c.quit)
		}

		if c.enc != nil {
			c.enc.flush()
			c.enc.close()
		}
		if c.conn != nil {
			c.conn.Close()
		}
	}
}
