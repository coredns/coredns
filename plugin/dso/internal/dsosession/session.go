package dsosession

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin/dso/internal/dsomessage"

	"github.com/miekg/dns"
)

var (
	ErrState       = fmt.Errorf("bad DSO state")
	ErrStateClosed = fmt.Errorf("%w - closed", ErrState)
)

// SessionGracefulCloseTimeout is amount of time before aborting after termination is initiated.
//
// RFC 8490, Section 6.6.1: After sending a DSO Retry Delay message, the server SHOULD
// allow the client five seconds to close the connection.
var SessionGracefulCloseTimeout = 5 * time.Second

type State uint32

const (
	// StateWaiting is state where session is waiting for establishing response:
	//  - Any DNS can be read or written
	//  - Only DSO requests can be read
	//  - Only DSO responses can be written
	StateWaiting State = iota
	// StatePending is state where session is handling writing establishing response:
	//  - Any DNS can be read or written
	//  - Any DSO can be read
	//  - Only DSO responses can be written
	StatePending
	// StateEstablished is state where session is successfully established:
	//  - Any DNS can be read or written
	//  - Any DSO can be read or written
	StateEstablished
	// StateClosing is state where session is closed but underlying connect may still functional:
	//  - Any DNS can be read or written
	//  - Any DSO can be read
	//  - No DSO can be written
	StateClosing
	// StateClosed is state where session is closed and underlying connection is non-functional:
	//  - No DNS can be read or written
	//  - No DSO can be read or written
	StateClosed
)

func (s State) String() string {
	switch s {
	case StateWaiting:
		return "waiting"
	case StatePending:
		return "pending"
	case StateEstablished:
		return "established"
	case StateClosing:
		return "closing"
	case StateClosed:
		return "closed"
	default:
		return strconv.FormatUint(uint64(s), 2)
	}
}

// Session implements server-side state management of RFC 8490 DNS Stateful Operations session.
//
//  1. Initially, state is [StateWaiting]
//  2. First written DSO response with RCODE=0 is establishing response, it changes state to [StatePending]
//  3. If write fails state is reset back to [StateWaiting]. Otherwise it's changed to [StateEstablished]
//  4. First written unidirectional with [dsomessage.RetryDelay] primary TLV initiates termination and changes state to [StateClosing]
//  5. Once connection is closed, state is changed to [StateClosed]
//
// Once session termination is initiated, no more messages can be written. If write of terminating message
// fails state remains [StateClosing] until [Session.Close] or [Session.Abort] is called.
type Session struct {
	Conn net.Conn

	closedC    chan struct{} // closed to signal that underlying connection is closed
	state      atomic.Uint32
	writeGroup sync.WaitGroup
}

// New creates new Session.
func New(conn net.Conn) (s *Session) {
	s = &Session{
		Conn:    conn,
		closedC: make(chan struct{}),
	}
	return s
}

// State returns current state.
func (s *Session) State() State {
	return State(s.state.Load())
}

// ReadMsg reads and returns one DNS message without length-prefix.
func (s *Session) ReadMsg(deadline time.Time) (msg []byte, err error) {
	s.Conn.SetReadDeadline(deadline)

	var length uint16
	if err = binary.Read(s.Conn, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	if length < dsomessage.MsgHeaderLen {
		return nil, dsomessage.ErrHeader
	}

	msg = make([]byte, length, max(length, readMsgBufCap))
	if _, err = io.ReadFull(s.Conn, msg); err != nil {
		return msg, err
	}

	raw := rawMsg(msg)
	// RFC 8490, Section 5.1: Until a DSO Session has been ... established,
	// a client MUST NOT initiate DSO unidirectional messages.
	//
	// RFC 8490, Section 5.5.2: If ... server receives a response (QR=1) ...
	// that does not match ... any of its outstanding operations, this is
	// a fatal error and the recipient MUST forcibly abort the connection immediately.
	if raw.opcode() == dns.OpcodeStateful && (raw.id() == 0 || raw.response()) && State(s.state.Load()) == StateWaiting {
		err = ErrState
	}

	return msg, err
}

// Write writes length-prefixed message and updates session's state if needed.
//
// Returns [ErrStateClosed] if session is closed, [ErrState] if it's in wrong state.
// Otherwise IO error is returned, if any.
//
// Session terminating [dsomessage.RetryDelay] unidirectional is recognized: it's last allowed message.
func (s *Session) Write(msg []byte) (n int, err error) {
	// Trust the caller to pass properly formatted message and use naive check for dispatching.

	raw := rawMsg(msg[dsomessage.LengthPrefixLen:])
	switch {
	case raw.opcode() != dns.OpcodeStateful:
		return s.writeDNS(msg)
	case raw.id() == 0 && raw.isRetryDelayMsg():
		return s.writeCloseUnidirectional(msg)
	case raw.id() == 0:
		return s.writeUnidirectional(msg)
	case !raw.response():
		return s.writeRequest(msg)
	case raw.rcode() != dns.RcodeSuccess:
		return s.writeErrorResponse(msg)
	default:
		return s.writeResponse(msg)
	}
}

// Close closes underlying connection.
func (s *Session) Close() error {
	return s.close(false)
}

// Abort forcibly closes underlying connection, e.g. with RST.
//
// Abort is implemented by calling NetConn() and SetLinger(0) on underlying connection, if possible.
func (s *Session) Abort() error {
	return s.close(true)
}

func (s *Session) writeDNS(msg []byte) (n int, err error) {
	s.writeGroup.Add(1)
	defer s.writeGroup.Done()

	switch State(s.state.Load()) {
	case StateWaiting:
		fallthrough
	case StatePending:
		fallthrough
	case StateEstablished:
		return s.Conn.Write(msg)
	case StateClosing:
		fallthrough
	case StateClosed:
		return 0, ErrStateClosed
	default:
		panic("dso.Session: unexpected DSO state")
	}
}

// doClose aborts session after [SessionGracefulCloseTimeout].
func (s *Session) doClose() {
	// Section 6.6.1: After sending a DSO Retry Delay message, the server SHOULD
	// allow the client five seconds to close the connection, and if the client
	// has not closed the connection after five seconds, then the server SHOULD
	// forcibly abort the connection.
	select {
	case <-time.After(SessionGracefulCloseTimeout):
		if State(s.state.Swap(uint32(StateClosed))) != StateClosed {
			abortConn(s.Conn)
			close(s.closedC)
		}
	case <-s.closedC:
		return
	}
}

// writeCloseUnidirectional initiates session termination.
//
// If session is not yet established, write is rejected with [ErrStateClosed]
// but intent is respected and session with underlying connection is closed.
// Otherwise [dsomessage.RetryDelay] unidirectional is written followed, if needed,
// by abort after [SessionGracefulCloseTimeout].
func (s *Session) writeCloseUnidirectional(msg []byte) (n int, err error) {
	var state State
	for {
		state = State(s.state.Load())
		if state == StateClosing || state == StateClosed {
			return 0, ErrStateClosed
		}
		if s.state.CompareAndSwap(uint32(state), uint32(StateClosing)) {
			break
		}
	}

	s.writeGroup.Wait()

	switch state {
	case StateWaiting:
		if State(s.state.Swap(uint32(StateClosed))) != StateClosed {
			s.state.Store(uint32(StateClosed))
			close(s.closedC)
			s.Conn.Close()
		}
		return 0, ErrStateClosed
	case StatePending:
		fallthrough
	case StateEstablished:
		n, err = s.Conn.Write(msg)
		if err == nil {
			go s.doClose()
		}
		return n, err
	default:
		panic("dso.Session: unexpected DSO state")
	}
}

func (s *Session) writeUnidirectional(msg []byte) (n int, err error) {
	s.writeGroup.Add(1)
	defer s.writeGroup.Done()

	switch State(s.state.Load()) {
	case StateWaiting:
		fallthrough
	case StatePending:
		return 0, ErrState
	case StateEstablished:
		return s.Conn.Write(msg)
	case StateClosing:
		fallthrough
	case StateClosed:
		return 0, ErrStateClosed
	default:
		panic("dso.Session: unexpected DSO state")
	}
}

func (s *Session) writeRequest(msg []byte) (n int, err error) {
	s.writeGroup.Add(1)
	defer s.writeGroup.Done()

	switch State(s.state.Load()) {
	case StateWaiting:
		fallthrough
	case StatePending:
		return 0, ErrState
	case StateEstablished:
		return s.Conn.Write(msg)
	case StateClosing:
		fallthrough
	case StateClosed:
		return 0, ErrStateClosed
	default:
		panic("dso.Session: unexpected DSO state")
	}
}

func (s *Session) writeErrorResponse(msg []byte) (n int, err error) {
	s.writeGroup.Add(1)
	defer s.writeGroup.Done()

	switch State(s.state.Load()) {
	case StateWaiting:
		fallthrough
	case StatePending:
		fallthrough
	case StateEstablished:
		return s.Conn.Write(msg)
	case StateClosing:
		fallthrough
	case StateClosed:
		return 0, ErrStateClosed
	default:
		panic("dso.Session: unexpected DSO state")
	}
}

func (s *Session) writeResponse(msg []byte) (n int, err error) {
	s.writeGroup.Add(1)
	defer s.writeGroup.Done()

	switch State(s.state.Load()) {
	case StateWaiting:
		if s.state.CompareAndSwap(uint32(StateWaiting), uint32(StatePending)) {
			n, err = s.Conn.Write(msg)
			if n == len(msg) {
				s.state.CompareAndSwap(uint32(StatePending), uint32(StateEstablished))
			} else {
				s.state.CompareAndSwap(uint32(StatePending), uint32(StateWaiting))
			}
			return n, err
		}
		fallthrough
	case StatePending:
		return 0, ErrState
	case StateEstablished:
		return s.Conn.Write(msg)
	case StateClosing:
		fallthrough
	case StateClosed:
		return 0, ErrStateClosed
	default:
		panic("dso.Session: unexpected DSO state")
	}
}

func (s *Session) close(abort bool) (err error) {
	switch State(s.state.Swap(uint32(StateClosed))) {
	case StateWaiting:
		fallthrough
	case StatePending:
		fallthrough
	case StateEstablished:
		fallthrough
	case StateClosing:
		if abort {
			abortConn(s.Conn)
		} else {
			err = s.Conn.Close()
		}
		close(s.closedC)
	case StateClosed:
	default:
		panic("dso.Session: unexpected DSO state")
	}
	return err
}

func abortConn(conn net.Conn) {
	netConn := conn
	if netConner, ok := netConn.(interface{ NetConn() net.Conn }); ok {
		netConn = netConner.NetConn()
	}
	if setLingerer, ok := netConn.(interface{ SetLinger(int) error }); ok {
		setLingerer.SetLinger(0)
	}
	netConn.Close()
}

type rawMsg []byte

func (m rawMsg) id() uint16 {
	return binary.BigEndian.Uint16(m)
}

func (m rawMsg) response() bool {
	return m[2]&0b1_0000_0_0_0 != 0
}

func (m rawMsg) opcode() uint16 {
	return uint16(m[2]&0b0_1111_0_0_0) >> 3
}

func (m rawMsg) rcode() uint16 {
	return uint16(m[3] & 0b0000_1111)
}

// isRetryDelayMsg naively determines whether DSO msg's primary TLV is RetryDelay.
func (m rawMsg) isRetryDelayMsg() bool {
	return len(m) >= dsomessage.MsgHeaderLen+dsomessage.TLVHeaderLen+dsomessage.RetryDelayLen &&
		dsomessage.Type(binary.BigEndian.Uint16(m[dsomessage.MsgHeaderLen:])) == dsomessage.TypeRetryDelay
}

const readMsgBufCap = 512 // enough to fit [dsomessage.TLSBlockLen]
