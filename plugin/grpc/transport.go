package grpc

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

const poolMaintainInterval = 5 * time.Second

// grpcConn wraps a gRPC connection with its client for shared access.
type grpcConn struct {
	conn     *grpc.ClientConn
	client   pb.DnsServiceClient
	lastUsed atomic.Int64 // unix nanoseconds; updated on every Dial hit
}

// touch records the current time as the last-used time for this connection.
func (gc *grpcConn) touch() { gc.lastUsed.Store(time.Now().UnixNano()) }

// idleFor returns how long this connection has been idle.
func (gc *grpcConn) idleFor() time.Duration {
	last := gc.lastUsed.Load()
	if last == 0 {
		return 0
	}
	return time.Since(time.Unix(0, last))
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
	expire   time.Duration // idle TTL; zero means no TTL eviction

	// Hot path: lock-free access via atomic pointer
	connList atomic.Pointer[connList]
	idx      atomic.Uint64

	// Cold path: mutex for modifications
	mu sync.Mutex

	// stopped is set before close(stop) to allow Dial to detect shutdown without a panic.
	stopped atomic.Bool
	// started tracks whether Start() has been called, to avoid deadlock in Stop().
	started atomic.Bool

	stop chan struct{}
	done chan struct{}

	proxyName string

	// Metrics counters (updated atomically, flushed periodically)
	hits   atomic.Uint64
	misses atomic.Uint64
}

// newTransport creates a new gRPC connection transport with shared connections.
func newTransport(proxyName, addr string, dialOpts []grpc.DialOption, poolSize int, expire time.Duration) *grpcTransport {
	t := &grpcTransport{
		addr:      addr,
		dialOpts:  dialOpts,
		poolSize:  poolSize,
		expire:    expire,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		proxyName: proxyName,
	}
	t.connList.Store(&connList{conns: make([]*grpcConn, 0, poolSize)})
	return t
}

// Dial returns a shared connection from the pool using round-robin.
// Returns ErrTransportStopped if the transport has been shut down.
func (t *grpcTransport) Dial(ctx context.Context) (*grpcConn, bool, error) {
	if t.stopped.Load() {
		return nil, false, ErrTransportStopped
	}

	list := t.connList.Load()
	n := len(list.conns)
	if n > 0 {
		idx := t.idx.Add(1)
		gc := list.conns[idx%uint64(n)]
		if gc != nil && gc.client != nil {
			gc.touch()
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
	gc.touch()

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

	return gc, nil
}

// addConn adds a connection to the pool. Returns false if the pool is already full.
func (t *grpcTransport) addConn(gc *grpcConn) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

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

	if gc.conn != nil {
		gc.conn.Close()
	}
}

// healthChecker periodically maintains the connection pool.
func (t *grpcTransport) healthChecker() {
	ticker := time.NewTicker(poolMaintainInterval)
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
		if !t.addConn(gc) {
			conn.Close() // pool filled concurrently; don't leak
			break
		}
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

// cleanupDeadConns removes connections that are in a failed gRPC state or have
// exceeded the idle TTL (expire duration).
func (t *grpcTransport) cleanupDeadConns() {
	t.mu.Lock()
	oldList := t.connList.Load()
	var toClose []*grpcConn
	newConns := make([]*grpcConn, 0, len(oldList.conns))

	for _, gc := range oldList.conns {
		if gc == nil || gc.conn == nil {
			continue
		}
		state := gc.conn.GetState()
		expired := t.expire > 0 && gc.idleFor() > t.expire
		if state == connectivity.Shutdown || state == connectivity.TransientFailure || expired {
			toClose = append(toClose, gc)
		} else {
			newConns = append(newConns, gc)
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

// Start starts the transport's connection pool maintenance goroutine.
func (t *grpcTransport) Start() {
	t.started.Store(true)
	go t.healthChecker()
}

// Stop shuts down the transport and closes all pooled connections.
func (t *grpcTransport) Stop() {
	t.stopped.Store(true)
	close(t.stop)
	// Only wait for healthChecker to exit if Start() was called.
	if t.started.Load() {
		<-t.done
	}

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
