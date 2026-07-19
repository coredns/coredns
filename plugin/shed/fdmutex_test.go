package shed

// Evidence tests for the fdMutex overflow panic described in README.md.
// One flood harness, two write disciplines:
//
//   - TestFdMutexPanicOneSlowWrite (SHED_FLOOD_TEST=1): one held write plus
//     >2^20 queued raw writers deterministically panic a subprocess.
//   - TestFdMutexPanicUDPFlood (SHED_FLOOD_TEST=1): the same panic with
//     nothing held — raw writers simply outpace the serialized drain.
//   - TestSingleWriterNoPanicSameLoad: the same responders through the
//     plugin's stack and single writer complete with every response written
//     or counted dropped. Runs at 50k responders by default (including
//     -race in CI); at the full 1.5M under SHED_FLOOD_TEST=1.
//
// The panic tests re-exec the test binary (the panic is a process death),
// cost ~1.5M goroutines / a few GiB / seconds, and assert on the runtime's
// message — env-gated so no automated or casual run pays that, or breaks if
// a future Go release rewords the panic.

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// overflowMsg must match GOROOT/src/internal/poll/fd_mutex.go.
const overflowMsg = "too many concurrent operations on a single file or socket (max 1048575)"

const (
	childEnv = "COREDNS_SHED_FDMUTEX_CHILD" // "flood", "held", absent = normal run
	floodEnv = "SHED_FLOOD_TEST"            // set to run the panic tests and the full-size survival test

	// fdMutex fields are 20-bit: the 1,048,576th concurrent op panics.
	fdMutexLimit = 1 << 20

	// floodWriters is comfortably above the limit, so the flood mode still
	// crosses it after subtracting whatever the drain completes while
	// spawning. ciWriters exercises the same code paths at a size every
	// test run can afford.
	floodWriters = 1_500_000
	ciWriters    = 50_000
	nSpawners    = 16

	// Flood mode uses near-max UDP payloads so each serialized sendto is
	// expensive — a stand-in for a response datapath slower than the
	// arrival rate.
	floodPayload = 63 * 1024

	childTimeout = 120 * time.Second
)

func TestMain(m *testing.M) {
	switch os.Getenv(childEnv) {
	case "flood":
		childFlood(false)
	case "held":
		childFlood(true)
	default:
		os.Exit(m.Run())
	}
	// The parent asserts on the exit status; this line is log-only.
	fmt.Println("CHILD-SURVIVED-WITHOUT-PANIC")
	os.Exit(0)
}

// spawnResponders spawns n goroutines, each calling respond once — a raw
// socket write in the panic modes, a stack push in the survival mode.
func spawnResponders(n int, respond func()) (started, completed *atomic.Int64) {
	started, completed = new(atomic.Int64), new(atomic.Int64)
	var spawn sync.WaitGroup
	for range nSpawners {
		spawn.Go(func() {
			for range n / nSpawners {
				started.Add(1)
				go func() {
					respond()
					completed.Add(1)
				}()
			}
		})
	}
	spawn.Wait()
	return started, completed
}

// childFlood is the crash payload: pile >2^20 concurrent raw writes onto one
// UDP socket. With held=true, one in-progress write is first parked via
// SyscallConn so the pile-up is deterministic; with held=false the writers
// race a genuine serialized drain.
func childFlood(held bool) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		fmt.Println("child: listen:", err)
		return
	}
	sink, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		fmt.Println("child: sink listen:", err)
		return
	}
	dst := sink.LocalAddr().(*net.UDPAddr)

	payload := make([]byte, floodPayload)
	if held {
		payload = payload[:64] // writes only queue as waiters; size is irrelevant
		// Park one write in progress: the callback holds the fd's write
		// lock exactly as a write blocked in the kernel would. Everything
		// arriving behind it becomes an fdMutex waiter.
		rc, err := conn.SyscallConn()
		if err != nil {
			fmt.Println("child: syscallconn:", err)
			return
		}
		holding := make(chan struct{})
		go func() {
			rc.Write(func(uintptr) bool {
				close(holding)
				select {} // hold the write lock for the life of the process
			})
		}()
		<-holding
		fmt.Println("child: one slow write in progress (fd write lock held)")
	}

	fmt.Printf("child: spawning %d concurrent UDP writers on one socket (limit %d)\n",
		floodWriters, fdMutexLimit-1)
	started, completed := spawnResponders(floodWriters, func() {
		conn.WriteToUDP(payload, dst) //nolint:errcheck // the pile, not the result, is the point
	})

	// If the panic is going to happen it already has (it fires inside a
	// writer's WriteToUDP). Give the drain a moment, then report survival.
	deadline := time.Now().Add(childTimeout)
	for completed.Load() < started.Load() && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
}

// runCrashChild re-execs this test binary in the given child mode and
// returns its combined output. The child is expected to die.
func runCrashChild(t *testing.T, mode string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), childEnv+"="+mode, "GOTRACEBACK=single")
	start := time.Now()
	out, err := cmd.CombinedOutput()
	t.Logf("child (%s) ran %v, err=%v", mode, time.Since(start).Round(time.Millisecond), err)
	s := string(out)
	// Panic output ends with a goroutine stack; keep the log readable.
	if i := strings.Index(s, "goroutine "); i > 0 {
		t.Logf("child output:\n%s[stack trace elided]", s[:i])
	} else {
		t.Logf("child output:\n%s", s)
	}
	if err == nil {
		t.Fatal("child process survived — expected fdMutex overflow panic")
	}
	return s
}

func skipUnlessFloodTest(t *testing.T) {
	t.Helper()
	if os.Getenv(floodEnv) == "" {
		t.Skipf("panic reproduction (~%d goroutines, a few GiB); set %s=1 to run", floodWriters, floodEnv)
	}
}

// TestFdMutexPanicUDPFlood: >2^20 genuinely concurrent writes on one UDP
// socket kill the process. Nothing is held or mocked — the writers simply
// arrive faster than the fd's serialized writes drain, which is the
// production storm condition.
func TestFdMutexPanicUDPFlood(t *testing.T) {
	skipUnlessFloodTest(t)
	out := runCrashChild(t, "flood")
	if !strings.Contains(out, overflowMsg) {
		t.Fatalf("child died without the fdMutex overflow panic; want %q", overflowMsg)
	}
}

// TestFdMutexPanicOneSlowWrite: deterministic variant — a single slow
// in-progress write plus >2^20 queued writers overflow the fdMutex waiter
// counter. No timing or throughput assumptions.
func TestFdMutexPanicOneSlowWrite(t *testing.T) {
	skipUnlessFloodTest(t)
	out := runCrashChild(t, "held")
	if !strings.Contains(out, overflowMsg) {
		t.Fatalf("child died without the fdMutex overflow panic; want %q", overflowMsg)
	}
}

// countingUDPWriter is the dns.Writer handed to the plugin's stack: the raw
// wire write, counted on success (a failed write is counted by the plugin
// as a drop).
type countingUDPWriter struct {
	conn    *net.UDPConn
	dst     *net.UDPAddr
	written atomic.Int64
}

func (w *countingUDPWriter) Write(p []byte) (int, error) {
	n, err := w.conn.WriteToUDP(p, w.dst)
	if err == nil {
		w.written.Add(1)
	}
	return n, err
}

// TestSingleWriterNoPanicSameLoad drives the flood harness through the
// plugin's actual respStack and writer goroutine. Only that one goroutine
// ever touches the fd, so the fdMutex overflow is structurally unreachable,
// and every response is accounted for as written or dropped.
func TestSingleWriterNoPanicSameLoad(t *testing.T) {
	n := ciWriters
	if os.Getenv(floodEnv) != "" {
		n = floodWriters
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	sink, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()

	dropped := droppedTotal.WithLabelValues(t.Name(), "response")
	droppedBefore := testutil.ToFloat64(dropped) // the child accumulates across -count>1 runs
	rs := newRespStack(stackDepth, dropped)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		rs.writerLoop()
	}()

	w := &countingUDPWriter{conn: conn, dst: sink.LocalAddr().(*net.UDPAddr)}
	payload := make([]byte, 64)

	start := time.Now()
	started, completed := spawnResponders(n, func() {
		// The responder's entire write path: what the decorator installs.
		(&stackWriter{stack: rs, inner: w}).Write(payload) //nolint:errcheck // always reports success
	})
	deadline := time.Now().Add(childTimeout)
	for completed.Load() < started.Load() {
		if time.Now().After(deadline) {
			t.Fatalf("only %d/%d responders completed", completed.Load(), started.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
	elapsed := time.Since(start)

	rs.close()
	<-writerDone

	written := w.written.Load()
	droppedN := int64(testutil.ToFloat64(dropped) - droppedBefore)
	if spawned := started.Load(); written+droppedN != spawned {
		t.Fatalf("accounting: written=%d + dropped=%d != %d responders", written, droppedN, spawned)
	}
	t.Logf("%d concurrent responders completed in %v with ONE fd writer: %d responses written, %d evicted (counted drops), no panic",
		started.Load(), elapsed.Round(time.Millisecond), written, droppedN)
}
