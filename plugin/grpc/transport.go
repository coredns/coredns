package grpc

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
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
type grpcTransport struct {
	addr     string
	dialOpts []grpc.DialOption
	poolSize int

	// Hot path: lock-free access via atomic pointer
	connList atomic.Pointer[connList]
	idx      atomic.Uint64

	// Cold path: mutex for modifications
	mu sync.Mutex

	stop chan struct{}
	done chan struct{}

	proxyName string

	// Metrics counters (updated atomically, flushed periodically)
	hits   atomic.Uint64
	misses atomic.Uint64
}

// newTransport creates a new gRPC connection transport with shared connections.
func newTransport(proxyName, addr string, dialOpts []grpc.DialOption, poolSize int, _ time.Duration) *grpcTransport {
	t := &grpcTransport{
		addr:      addr,
		dialOpts:  dialOpts,
		poolSize:  poolSize,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		proxyName: proxyName,
	}
	t.connList.Store(&connList{conns: make([]*grpcConn, 0, poolSize)})
	return t
}

// Dial returns a shared connection from the pool using round-robin.
func (t *grpcTransport) Dial(ctx context.Context) (*grpcConn, bool, error) {
	list := t.connList.Load()
	n := len(list.conns)
	if n > 0 {
		idx := t.idx.Add(1)
		gc := list.conns[idx%uint64(n)]
		if gc != nil && gc.client != nil {
			t.hits.Add(1)
			return gc, false, nil
		}
	}

	t.misses.Add(1)
	gc, err := t.createConn(ctx)
	if err != nil {
		return nil, false, err
	}

	return gc, true, nil
}

// createConn creates a new gRPC connection and adds it to the pool.
func (t *grpcTransport) createConn(ctx context.Context) (*grpcConn, error) {
	conn, err := grpc.NewClient(t.addr, t.dialOpts...)
	if err != nil {
		return nil, err
	}

	gc := &grpcConn{
		conn:   conn,
		client: pb.NewDnsServiceClient(conn),
	}

	t.addConn(gc)
	return gc, nil
}

// addConn adds a connection to the pool.
func (t *grpcTransport) addConn(gc *grpcConn) {
	t.mu.Lock()
	defer t.mu.Unlock()

	oldList := t.connList.Load()
	if len(oldList.conns) >= t.poolSize {
		return
	}

	newConns := make([]*grpcConn, len(oldList.conns)+1)
	copy(newConns, oldList.conns)
	newConns[len(oldList.conns)] = gc
	t.connList.Store(&connList{conns: newConns})
}

// Yield is a no-op for shared connections.
func (t *grpcTransport) Yield(gc *grpcConn) {}

// RemoveConn removes a failed connection from the pool.
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

	if gc.conn != nil {
		gc.conn.Close()
	}
}

// healthChecker periodically checks connection health and maintains the pool.
func (t *grpcTransport) healthChecker() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	defer close(t.done)

	t.prewarm()

	for {
		select {
		case <-ticker.C:
			t.cleanupDeadConns()
			t.flushMetrics()
			t.prewarm()
		case <-t.stop:
			return
		}
	}
}

// prewarm creates connections up to pool size.
func (t *grpcTransport) prewarm() {
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
		t.addConn(gc)
	}
}

// flushMetrics updates Prometheus metrics from atomic counters.
func (t *grpcTransport) flushMetrics() {
	hits := t.hits.Swap(0)
	misses := t.misses.Swap(0)
	if hits > 0 {
		PoolHitsCount.WithLabelValues(t.addr).Add(float64(hits))
	}
	if misses > 0 {
		PoolMissesCount.WithLabelValues(t.addr).Add(float64(misses))
	}
	list := t.connList.Load()
	PoolSizeGauge.WithLabelValues(t.addr).Set(float64(len(list.conns)))
}

// cleanupDeadConns removes connections in failed state.
func (t *grpcTransport) cleanupDeadConns() {
	t.mu.Lock()
	oldList := t.connList.Load()
	var toClose []*grpcConn
	newConns := make([]*grpcConn, 0, len(oldList.conns))

	for _, gc := range oldList.conns {
		if gc != nil && gc.conn != nil {
			state := gc.conn.GetState()
			if state == connectivity.Shutdown || state == connectivity.TransientFailure {
				toClose = append(toClose, gc)
			} else {
				newConns = append(newConns, gc)
			}
		}
	}

	if len(toClose) > 0 {
		t.connList.Store(&connList{conns: newConns})
	}
	t.mu.Unlock()

	for _, gc := range toClose {
		gc.conn.Close()
	}
}

// Start starts the transport's health checker.
func (t *grpcTransport) Start() {
	go t.healthChecker()
}

// Stop stops the transport and closes all connections.
func (t *grpcTransport) Stop() {
	close(t.stop)
	<-t.done

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

var (
	ErrTransportStopped = &transportError{"transport stopped"}
)

type transportError struct {
	msg string
}

func (e *transportError) Error() string {
	return e.msg
}
