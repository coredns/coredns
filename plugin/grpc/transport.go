package grpc

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/coredns/coredns/pb"

	"google.golang.org/grpc"
)

// grpcConn wraps a gRPC connection with its client for shared access.
type grpcConn struct {
	conn   *grpc.ClientConn
	client pb.DnsServiceClient
}

// connList is an immutable slice of connections for lock-free access.
type connList struct {
	conns []*grpcConn
}

// grpcTransport manages a pool of shared gRPC connections to a single upstream.
// Uses lock-free atomic operations on the hot path for maximum throughput.
//
// gRPC ClientConn manages its own connection lifecycle: it reconnects
// automatically on failure and multiplexes RPCs across a single connection.
// No background maintenance goroutine is needed.
type grpcTransport struct {
	addr     string
	dialOpts []grpc.DialOption
	poolSize int

	// Hot path: lock-free access via atomic pointer.
	connList atomic.Pointer[connList]
	idx      atomic.Uint64

	// Cold path: mutex for modifications.
	mu sync.Mutex

	// stopped is set in Stop to prevent new connections from being added after shutdown.
	stopped atomic.Bool

	proxyName string
}

// newTransport creates a new gRPC connection transport with shared connections.
func newTransport(proxyName, addr string, dialOpts []grpc.DialOption, poolSize int) *grpcTransport {
	t := &grpcTransport{
		addr:      addr,
		dialOpts:  dialOpts,
		poolSize:  poolSize,
		proxyName: proxyName,
	}
	t.connList.Store(&connList{conns: make([]*grpcConn, 0, poolSize)})
	return t
}

// Dial returns a shared connection from the pool using round-robin.
// Returns ErrTransportStopped if the transport has been shut down.
func (t *grpcTransport) Dial(ctx context.Context) (*grpcConn, error) {
	if t.stopped.Load() {
		return nil, ErrTransportStopped
	}

	list := t.connList.Load()
	n := len(list.conns)
	if n > 0 {
		idx := t.idx.Add(1)
		gc := list.conns[idx%uint64(n)]
		if gc != nil && gc.client != nil {
			PoolHitsCount.WithLabelValues(t.addr).Inc()
			return gc, nil
		}
	}

	PoolMissesCount.WithLabelValues(t.addr).Inc()
	return t.createConn(ctx)
}

// createConn creates a new gRPC connection and attempts to add it to the pool.
// If the pool is already full, the new connection is closed and an existing
// pool connection is returned to avoid leaking resources.
func (t *grpcTransport) createConn(ctx context.Context) (*grpcConn, error) {
	conn, err := grpc.NewClient(t.addr, t.dialOpts...)
	if err != nil {
		return nil, err
	}

	gc := &grpcConn{
		conn:   conn,
		client: pb.NewDnsServiceClient(conn),
	}

	if !t.addConn(gc) {
		// Pool is full (concurrent creation race). Close the new connection
		// to avoid leaking it and return an existing one from the pool.
		conn.Close()
		list := t.connList.Load()
		if n := len(list.conns); n > 0 {
			return list.conns[t.idx.Add(1)%uint64(n)], nil
		}
		// Shouldn't happen: pool was full but is now empty.
		return nil, ErrTransportStopped
	}

	PoolSizeGauge.WithLabelValues(t.addr).Inc()
	return gc, nil
}

// addConn adds a connection to the pool. Returns false if the pool is already full
// or the transport has been stopped.
func (t *grpcTransport) addConn(gc *grpcConn) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stopped.Load() {
		return false
	}

	oldList := t.connList.Load()
	if len(oldList.conns) >= t.poolSize {
		return false
	}

	newConns := make([]*grpcConn, len(oldList.conns)+1)
	copy(newConns, oldList.conns)
	newConns[len(oldList.conns)] = gc
	t.connList.Store(&connList{conns: newConns})
	return true
}

// RemoveConn removes a failed connection from the pool and closes it.
func (t *grpcTransport) RemoveConn(gc *grpcConn) {
	if gc == nil {
		return
	}

	t.mu.Lock()
	oldList := t.connList.Load()
	newConns := make([]*grpcConn, 0, len(oldList.conns))
	for _, c := range oldList.conns {
		if c != gc {
			newConns = append(newConns, c)
		}
	}
	t.connList.Store(&connList{conns: newConns})
	t.mu.Unlock()

	PoolSizeGauge.WithLabelValues(t.addr).Dec()

	if gc.conn != nil {
		gc.conn.Close()
	}
}

// Start pre-populates the pool up to poolSize.
// grpc.NewClient is lazy: no TCP connection is established until the first RPC,
// so this just pre-allocates ClientConn objects to avoid createConn overhead on
// the first few requests.
func (t *grpcTransport) Start() {
	list := t.connList.Load()
	needed := t.poolSize - len(list.conns)
	for range needed {
		conn, err := grpc.NewClient(t.addr, t.dialOpts...)
		if err != nil {
			continue
		}
		gc := &grpcConn{
			conn:   conn,
			client: pb.NewDnsServiceClient(conn),
		}
		if !t.addConn(gc) {
			conn.Close() // pool filled concurrently; don't leak
			break
		}
		PoolSizeGauge.WithLabelValues(t.addr).Inc()
	}
}

// Stop shuts down the transport and closes all pooled connections.
func (t *grpcTransport) Stop() {
	t.stopped.Store(true)

	t.mu.Lock()
	list := t.connList.Load()
	t.connList.Store(&connList{conns: nil})
	t.mu.Unlock()

	for _, gc := range list.conns {
		if gc != nil && gc.conn != nil {
			gc.conn.Close()
		}
	}
	PoolSizeGauge.WithLabelValues(t.addr).Set(0)
}

// ErrTransportStopped is returned by Dial when the transport has been shut down.
var ErrTransportStopped = errors.New("transport stopped")
