package dsosession

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coredns/coredns/plugin/dso/internal/dsomessage"

	"github.com/google/go-cmp/cmp"
	"github.com/miekg/dns"
)

const noMessageTimeout = 500 * time.Millisecond

var (
	establishingMsg = dsomessage.NewRepMsg(9000, &dsomessage.KeepAlive{})
	errorMsg        = dsomessage.NewErrorMsg(1, dns.RcodeStatefulTypeNotImplemented)
	repMsg          = dsomessage.NewRepMsg(1, &dsomessage.KeepAlive{})
	reqMsg          = dsomessage.NewReqMsg(1, &dsomessage.KeepAlive{})
	uniMsg          = dsomessage.NewUniMsg(&dsomessage.KeepAlive{})
	closeMsg        = dsomessage.NewUniMsg(&dsomessage.RetryDelay{})

	queryMsg = new(dns.Msg).SetQuestion("test.", dns.TypeA)
)

type packer interface {
	Pack() ([]byte, error)
}

// packMsg packs [dns.Msg] and [dsomessage.Msg] for wire.
func packMsg(tb testing.TB, m packer) []byte {
	tb.Helper()

	msg, err := m.Pack()
	if err != nil {
		tb.Fatalf("Got Pack()=%v", err)
	}
	msg = append(msg, 0, 0)
	copy(msg[2:], msg)
	binary.BigEndian.PutUint16(msg, uint16(len(msg)-2))
	return msg
}

// testSession is utility wrapper over [Session].
//
// Use with [testing/synctest].
type testSession struct {
	*Session
	clientConn net.Conn
}

func newTestSession() *testSession {
	serverConn, clientConn := net.Pipe()
	return &testSession{
		Session:    New(serverConn),
		clientConn: clientConn,
	}
}

func (s *testSession) assertState(tb testing.TB, want State) {
	tb.Helper()

	synctest.Wait()
	if got := s.State(); got != want {
		tb.Fatalf("Got State()=%v, want %v", got, want)
	}
}

// assertClientReadMsg asserts that client reads given message.
func (s *testSession) assertClientReadMsg(tb testing.TB, m packer) {
	tb.Helper()

	want := packMsg(tb, m)
	got := make([]byte, len(want))
	_, err := s.clientConn.Read(got)

	if err != nil {
		tb.Fatalf("Got %v, want client to read message", err)
	}
	if diff := cmp.Diff(want, got); len(diff) > 0 {
		tb.Fatalf("%v", diff)
	}
}

func (s *testSession) assertClientReadMsgFails(tb testing.TB) {
	tb.Helper()

	n, _ := io.Copy(io.Discard, s.clientConn)
	if n > 0 {
		tb.Fatalf("Got n=%v bytes, want 0", n)
	}
}

// assertWriteMsg asserts that server writes given message.
func (s *testSession) assertWriteMsg(tb testing.TB, m packer) {
	tb.Helper()

	var (
		want = packMsg(tb, m)
		got  = make([]byte, len(want))
		err  error
		wg   sync.WaitGroup
	)
	wg.Go(func() {
		_, err = s.Write(want)
	})
	wg.Go(func() {
		s.clientConn.Read(got)
	})
	wg.Wait()

	if err != nil {
		tb.Fatalf("Got Write()=%v", err)
	}
	if diff := cmp.Diff(want, got); len(diff) > 0 {
		tb.Fatalf("%v", diff)
	}
}

func (s *testSession) assertWriteMsgFails(tb testing.TB, m packer, want error) {
	tb.Helper()

	var (
		msg = packMsg(tb, m)
		wg  sync.WaitGroup
	)

	wg.Go(func() {
		s.clientConn.SetReadDeadline(time.Now().Add(noMessageTimeout))
		io.Copy(io.Discard, s.clientConn)
		s.clientConn.SetReadDeadline(time.Time{})
	})
	_, got := s.Write(msg)
	wg.Wait()

	if got == nil || !errors.Is(got, want) {
		tb.Fatalf("Got %v, want %v", got, want)
	}
}

func setupSession(tb testing.TB) (sesh *testSession) {
	tb.Helper()

	sesh = newTestSession()
	tb.Cleanup(func() {
		sesh.Conn.Close()
		sesh.clientConn.Close()
	})
	tb.Cleanup(func() {
		sesh.Close()
	})
	return sesh
}

func setupWaitingSession(tb testing.TB) *testSession {
	tb.Helper()

	sesh := setupSession(tb)
	sesh.assertState(tb, StateWaiting)
	return sesh
}

func setupPendingSession(tb testing.TB) *testSession {
	tb.Helper()

	sesh := setupSession(tb)
	go sesh.Write(packMsg(tb, establishingMsg)) // remains pending until clientConn reads
	sesh.assertState(tb, StatePending)
	return sesh
}

func setupEstablishedSession(tb testing.TB) *testSession {
	tb.Helper()

	sesh := setupSession(tb)
	sesh.assertWriteMsg(tb, establishingMsg)
	sesh.assertState(tb, StateEstablished)
	return sesh
}

// Note session remains in closing state up until [SessionGracefulCloseTimeout].
func setupClosingSession(tb testing.TB) *testSession {
	tb.Helper()

	sesh := setupEstablishedSession(tb)
	sesh.assertWriteMsg(tb, closeMsg)
	sesh.assertState(tb, StateClosing)
	return sesh
}

func setupClosedSession(tb testing.TB) *testSession {
	tb.Helper()

	sesh := setupWaitingSession(tb)
	sesh.Close()
	sesh.assertState(tb, StateClosed)
	return sesh
}

func TestSessionState(t *testing.T) {
	tcs := []struct {
		name      string
		setupFunc func(testing.TB) *testSession
	}{
		{
			StateWaiting.String(),
			setupWaitingSession,
		},
		{
			StatePending.String(),
			setupPendingSession,
		},
		{
			StateEstablished.String(),
			setupEstablishedSession,
		},
		{
			StateClosing.String(),
			setupClosingSession,
		},
		{
			StateClosed.String(),
			setupClosedSession,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				tc.setupFunc(t)
			})
		})
	}
}

func TestSessionWrite(t *testing.T) {
	t.Parallel()

	t.Run(StateWaiting.String(), func(t *testing.T) {
		t.Parallel()

		tcs := []struct {
			name    string
			m       packer
			wantErr error
		}{
			{"error rep", errorMsg, nil},
			{"rep", repMsg, nil},
			{"req", reqMsg, ErrState},
			{"uni", uniMsg, ErrState},
			{"dns", queryMsg, nil},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					sesh := setupWaitingSession(t)
					if tc.wantErr == nil {
						sesh.assertWriteMsg(t, tc.m)
					} else {
						sesh.assertWriteMsgFails(t, tc.m, tc.wantErr)
					}
				})
			})
		}
	})

	t.Run(StatePending.String(), func(t *testing.T) {
		t.Parallel()

		tcs := []struct {
			name    string
			m       packer
			wantErr error
		}{
			{"error rep", errorMsg, nil},
			{"rep", repMsg, ErrState},
			{"req", reqMsg, ErrState},
			{"uni", uniMsg, ErrState},
			{"dns", queryMsg, nil},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					sesh := setupPendingSession(t)
					if tc.wantErr == nil {
						var wg sync.WaitGroup
						wg.Go(func() {
							sesh.assertWriteMsg(t, tc.m)
						})
						sesh.assertClientReadMsg(t, establishingMsg)
						wg.Wait()
					} else {
						sesh.assertWriteMsgFails(t, tc.m, tc.wantErr)
					}
				})
			})
		}
	})

	t.Run(StateEstablished.String(), func(t *testing.T) {
		t.Parallel()

		tcs := []struct {
			name string
			m    packer
		}{
			{"error rep", errorMsg},
			{"rep", repMsg},
			{"req", reqMsg},
			{"uni", uniMsg},
			{"dns", queryMsg},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					sesh := setupEstablishedSession(t)
					sesh.assertWriteMsg(t, tc.m)
				})
			})
		}
	})

	t.Run(StateClosing.String(), func(t *testing.T) {
		t.Parallel()

		tcs := []struct {
			name    string
			m       packer
			wantErr error
		}{
			{"error rep", errorMsg, ErrStateClosed},
			{"rep", repMsg, ErrStateClosed},
			{"req", reqMsg, ErrStateClosed},
			{"uni", uniMsg, ErrStateClosed},
			{"dns", queryMsg, ErrStateClosed},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					sesh := setupClosingSession(t)
					sesh.assertWriteMsgFails(t, tc.m, tc.wantErr)
				})
			})
		}
	})

	t.Run(StateClosed.String(), func(t *testing.T) {
		t.Parallel()

		tcs := []struct {
			name    string
			m       packer
			wantErr error
		}{
			{"error rep", errorMsg, ErrStateClosed},
			{"rep", repMsg, ErrStateClosed},
			{"req", reqMsg, ErrStateClosed},
			{"uni", uniMsg, ErrStateClosed},
			{"dns", queryMsg, ErrStateClosed},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					sesh := setupClosedSession(t)
					sesh.assertWriteMsgFails(t, tc.m, tc.wantErr)
				})
			})
		}
	})
}

func TestSessionCloseNotify(t *testing.T) {
	t.Parallel()

	t.Run(StateWaiting.String(), func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sesh := setupWaitingSession(t)
			sesh.assertWriteMsgFails(t, closeMsg, ErrStateClosed)
			sesh.assertState(t, StateClosed)
		})
	})

	t.Run(StatePending.String(), func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sesh := setupPendingSession(t)
			var wg sync.WaitGroup
			wg.Go(func() {
				sesh.assertWriteMsg(t, closeMsg)
			})
			sesh.assertClientReadMsg(t, establishingMsg)
			wg.Wait()
			sesh.assertState(t, StateClosing)
		})
	})

	t.Run(StateEstablished.String(), func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sesh := setupEstablishedSession(t)
			sesh.assertWriteMsg(t, closeMsg)
			sesh.assertState(t, StateClosing)
		})
	})

	t.Run(StateClosing.String(), func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sesh := setupClosingSession(t)
			sesh.assertWriteMsgFails(t, closeMsg, ErrStateClosed)
			sesh.assertState(t, StateClosing)
		})
	})

	t.Run(StateClosed.String(), func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sesh := setupClosedSession(t)
			sesh.assertWriteMsgFails(t, closeMsg, ErrStateClosed)
			sesh.assertState(t, StateClosed)
		})
	})
}

func TestSessionClose(t *testing.T) {
	t.Parallel()

	t.Run(StateWaiting.String(), func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sesh := setupWaitingSession(t)
			sesh.Close()
			sesh.assertState(t, StateClosed)
		})
	})

	t.Run(StatePending.String(), func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sesh := setupPendingSession(t)
			sesh.Close()
			sesh.assertClientReadMsgFails(t)
			sesh.assertState(t, StateClosed)
		})
	})

	t.Run(StateEstablished.String(), func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sesh := setupEstablishedSession(t)
			sesh.Close()
			sesh.assertState(t, StateClosed)
		})
	})

	t.Run(StateClosing.String(), func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sesh := setupClosingSession(t)
			sesh.Close()
			sesh.assertState(t, StateClosed)
		})
	})

	t.Run(StateClosed.String(), func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sesh := setupClosedSession(t)
			sesh.Close()
			sesh.assertState(t, StateClosed)
		})
	})
}

type (
	testNetConn struct {
		net.Conn
		called atomic.Bool
	}

	testSetLingerConn struct {
		net.Conn
		called atomic.Bool
	}
)

func (conn *testNetConn) NetConn() net.Conn {
	conn.called.Store(true)
	return conn.Conn
}

func (conn *testSetLingerConn) SetLinger(n int) error {
	if n == 0 {
		conn.called.Store(true)
	}
	return nil
}

func TestSessionTimeoutForcedClose(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		sesh := setupEstablishedSession(t)
		slc := testSetLingerConn{Conn: sesh.Conn}
		nc := testNetConn{Conn: &slc}
		sesh.Conn = &nc
		sesh.assertWriteMsg(t, closeMsg)

		time.Sleep(SessionGracefulCloseTimeout + 1)

		sesh.assertState(t, StateClosed)

		if !nc.called.Load() {
			t.Error("Want NetConn() called")
		}
		if !slc.called.Load() {
			t.Error("Want SetLinger(0) called")
		}
	})
}

func TestSessionDoCloseExitsOnClose(t *testing.T) {
	t.Parallel()

	t.Run("close", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sesh := setupClosingSession(t)

			t1 := time.Now()
			sesh.Close()
			synctest.Wait()
			sesh.assertState(t, StateClosed)
			if time.Since(t1) >= SessionGracefulCloseTimeout {
				t.Error("Want to close before timeout")
			}
		})
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sesh := setupClosingSession(t)
			time.Sleep(SessionGracefulCloseTimeout - time.Nanosecond)
			sesh.assertState(t, StateClosing)
			time.Sleep(time.Nanosecond)
			sesh.assertState(t, StateClosed)
		})
	})
}

func TestSessionRead(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name    string
		length  uint16
		msg     []byte
		wantErr error
	}{
		{
			"short",
			dsomessage.MsgHeaderLen - 1,
			nil,
			dsomessage.ErrHeader,
		},
		{
			"timeout",
			dsomessage.MsgHeaderLen,
			nil,
			os.ErrDeadlineExceeded,
		},
		{
			"ok",
			dsomessage.MsgHeaderLen,
			make([]byte, dsomessage.MsgHeaderLen),
			nil,
		},
		{
			"bad response",
			dsomessage.MsgHeaderLen,
			[]byte{0, 1, 0b1_0110_000, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ErrState,
		},
		{
			"bad unidirectional",
			dsomessage.MsgHeaderLen,
			[]byte{0, 0, 0b0_0110_000, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ErrState,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				sesh := setupWaitingSession(t)

				msg := binary.BigEndian.AppendUint16(nil, tc.length)
				msg = append(msg, tc.msg...)

				var wg sync.WaitGroup
				wg.Go(func() {
					sesh.clientConn.Write(msg)
				})

				_, err := sesh.ReadMsg(time.Now().Add(noMessageTimeout))
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("Got ReadMsg()=%v, want %v", err, tc.wantErr)
				}

				wg.Wait()
			})
		})
	}
}

func FuzzSession(f *testing.F) {
	f.Fuzz(func(t *testing.T,
		reqN uint8,
		repN uint8,
		uniN uint8,
		closeN uint8,
		dnsN uint8,

		close bool,
		abort bool,

		seed uint64,
	) {
		synctest.Test(t, func(t *testing.T) {
			sesh := setupWaitingSession(t)
			defer sesh.Close()

			var (
				pcg = rand.NewPCG(seed, 0)
				r   = rand.New(pcg)

				wg sync.WaitGroup
			)

			wg.Go(func() {
				io.Copy(io.Discard, sesh.clientConn)
			})

			for i := range reqN {
				delay := r.Uint32()
				wg.Go(func() {
					time.Sleep(time.Duration(delay))

					m := dsomessage.NewReqMsg(uint16(i), &dsomessage.KeepAlive{})
					sesh.Write(packMsg(t, m))
				})
			}
			for i := range repN {
				delay := r.Uint32()
				wg.Go(func() {
					time.Sleep(time.Duration(delay))

					var m *dsomessage.Msg
					if i%2 == 0 {
						m = dsomessage.NewRepMsg(uint16(i), &dsomessage.KeepAlive{})
					} else {
						m = dsomessage.NewErrorMsg(uint16(i), dns.RcodeRefused, &dsomessage.RetryDelay{})
					}
					sesh.Write(packMsg(t, m))
				})
			}
			for range uniN {
				delay := r.Uint32()
				wg.Go(func() {
					time.Sleep(time.Duration(delay))

					m := dsomessage.NewUniMsg(&dsomessage.KeepAlive{})
					sesh.Write(packMsg(t, m))
				})
			}
			for range closeN {
				delay := r.Uint32()
				wg.Go(func() {
					time.Sleep(time.Duration(delay))

					m := dsomessage.NewUniMsg(&dsomessage.RetryDelay{})
					sesh.Write(packMsg(t, m))
				})
			}
			for i := range dnsN {
				delay := r.Uint32()
				wg.Go(func() {
					time.Sleep(time.Duration(delay))

					m := new(dns.Msg).SetQuestion(fmt.Sprintf("x%d.test.", i), dns.TypeA)
					sesh.Write(packMsg(t, m))
				})
			}

			if close {
				delay := r.Uint32()
				wg.Go(func() {
					time.Sleep(time.Duration(delay))
					sesh.Close()
				})
			}

			if abort {
				delay := r.Uint32()
				wg.Go(func() {
					time.Sleep(time.Duration(delay))
					sesh.Abort()
				})
			}

			wg.Wait()
		})
	})
}
