package dso

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// SessionState is the list of session states.
type SessionState uint32

// Possible transitions:
// SessionWaiting -> SessionClosed (RetryDelay unidirectional)
// SessionWaiting -> sessionPending -> SessionClosed (writing error)
// SessionWaiting -> sessionPending -> SessionEstablished -> SessionClosed (RetryDelay unidirectional)
const (
	// The session is waiting for an establishing exchange.
	SessionWaiting SessionState = 0
	// The session is being established.
	sessionPending SessionState = 1 << iota
	// The session is successfully established.
	SessionEstablished
	// The session is closed and no longer accepts new messages.
	SessionClosed
)

// Session implements state management of RFC 8490 DNS Stateful Operations session.
type Session struct {
	idleTimeout time.Duration

	state     atomic.Uint32
	mu        *sync.RWMutex // serialize writes of close vs all other
	pendingCh chan struct{} // closed if state is (SessionEstablished | SessionClosed)

	ka *dns.DSOKeepAlive
}

// NewSession instantiates new Session.
//
// idleTimeout is used to derive appropriate values for the KeepAlive TLV.
func NewSession(idleTimeout time.Duration) *Session {
	s := new(Session)
	s.idleTimeout = idleTimeout
	s.mu = new(sync.RWMutex)
	s.pendingCh = make(chan struct{})
	return s
}

// NewClosedSession instantiates new closed Session.
func NewClosedSession(idleTimeout time.Duration) *Session {
	s := new(Session)
	s.idleTimeout = idleTimeout
	s.state.Store(uint32(SessionClosed))
	// s.mu is left uninitialized
	// s.pendingCh is left uninitialized
	return s
}

// State returns current session state.
//
// Blocks while the session is pending.
func (s *Session) State() SessionState {
	state := SessionState(s.state.Load())
	if state == SessionClosed {
		return SessionClosed
	}
	if state == sessionPending {
		<-s.pendingCh
		state = SessionState(s.state.Load())
	}
	return state
}

// IsValidState checks whether the session state is valid to handle the DSO message.
//
// Returns ErrSessionClosed if session is closed, ErrSessionState if it's in the wrong state.
func (s *Session) IsValidState(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) error {
	state := SessionState(s.state.Load())
	if state == SessionClosed {
		return ErrSessionClosed
	}
	if state != SessionEstablished && !dns.IsDSOResponse(r) {
		return ErrSessionState
	}
	return nil
}

// WriteMsg writes message and updates associated session state if necessary.
//
// Returns ErrSessionClosed if the connection is closed, ErrSessionState if the message is not appropriate
// for current state or the writing error.
// Session is considered closed on writing error.
func (s *Session) WriteMsg(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (err error) {
	// Messages can be written concurrently with the following exceptions:
	// 1. Requests, unidirectional and consequent successful responses must wait
	//    until the 1st successful reponse is sent
	// 2. No messages can be written after Close unidirectional
	isCloseUnidirectional := dns.IsDSOUnidirectional(r) && r.Stateful[0].DSOType() == dns.StatefulTypeRetryDelay
	var locker sync.Locker
	if isCloseUnidirectional {
		locker = s.mu
	} else {
		locker = s.mu.RLocker()
	}

	locker.Lock()
	defer locker.Unlock()

	switch {
	case isCloseUnidirectional:
		err = s.writeCloseUnidirectional(ctx, w, r)
	case dns.IsDSOUnidirectional(r):
		err = s.writeUnidirectional(ctx, w, r)
	case dns.IsDSORequest(r):
		err = s.writeRequest(ctx, w, r)
	default:
		err = s.writeResponse(ctx, w, r)
	}

	if err != nil && !errors.Is(err, ErrSessionState) {
		// Close session on writing error.
		s.state.Store(uint32(SessionClosed))
	}

	return nil
}

func (s *Session) writeCloseUnidirectional(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (err error) {
	// isCloseUnidirectional has exclusive access and thus session cannot be in transient state.
	switch SessionState(s.state.Load()) {
	case SessionWaiting:
		s.state.Store(uint32(SessionClosed))
		close(s.pendingCh)
		return nil
	case SessionEstablished:
		s.state.Store(uint32(SessionClosed)) // set state early, before waiting for write
		return w.WriteMsg(r)
	case SessionClosed:
		return nil
	default:
		panic("unexpected DSO state")
	}
}

func (s *Session) writeUnidirectional(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (err error) {
	state := SessionState(s.state.Load())
	if state == sessionPending {
		<-s.pendingCh
		state = SessionState(s.state.Load())
	}

	switch state {
	case SessionWaiting:
		return ErrSessionState
	case SessionEstablished:
		return w.WriteMsg(r)
	case SessionClosed:
		return ErrSessionClosed
	default:
		panic("unexpected DSO state")
	}
}

func (s *Session) writeRequest(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (err error) {
	state := SessionState(s.state.Load())
	if state == sessionPending {
		<-s.pendingCh
		state = SessionState(s.state.Load())
	}

	switch state {
	case SessionWaiting:
		return ErrSessionState
	case SessionEstablished:
		return w.WriteMsg(r)
	case SessionClosed:
		return ErrSessionClosed
	default:
		panic("unexpected DSO state")
	}
}

func (s *Session) writeResponse(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (err error) {
	state := SessionState(s.state.Load())
	isEstablishingResponse := state == SessionWaiting && r.Rcode == dns.RcodeSuccess
	if isEstablishingResponse && !s.state.CompareAndSwap(uint32(SessionWaiting), uint32(sessionPending)) {
		isEstablishingResponse = false
		<-s.pendingCh
		state = SessionState(s.state.Load())
	}

	switch state {
	case SessionWaiting:
		fallthrough
	case SessionEstablished:
		err = w.WriteMsg(r)
		if isEstablishingResponse {
			switch {
			case err == nil && s.state.CompareAndSwap(uint32(sessionPending), uint32(SessionEstablished)):
				fallthrough
			case err != nil && s.state.CompareAndSwap(uint32(sessionPending), uint32(SessionClosed)):
				close(s.pendingCh)
			}
			if err == nil && (len(r.Stateful) == 0 || r.Stateful[0].DSOType() != dns.StatefulTypeKeepAlive) {
				// The session is established via a non-KeepAlive exchange. Tell the client our timeouts.
				m := new(dns.Msg)
				dns.SetDSOUnidirectional(m)
				m.Stateful = append(m.Stateful, s.DefaultKeepAlive())
				err = w.WriteMsg(m)
			}
		}
		return err
	case SessionClosed:
		return ErrSessionClosed
	default:
		panic("unexpected DSO state")
	}
}

// Abort forcibly aborts the connection.
func (s *Session) Abort(ctx context.Context, w dns.ResponseWriter) {
	w.(dns.ResponseWriterExtra).Abort()
}

// DefaultKeepAlive returns default KeepAlive TLV configured with server's timeouts.
//
// You can adjust the values before the first use.
func (s *Session) DefaultKeepAlive() *dns.DSOKeepAlive {
	if s.ka == nil {
		s.ka = &dns.DSOKeepAlive{
			InactivityTimeout: uint32(max(0*time.Second, min(s.idleTimeout-5*time.Second, s.idleTimeout/2)).Milliseconds()),
			KeepAliveInterval: uint32(max(dns.DSOKeepAliveIntervalMin, s.idleTimeout/2).Milliseconds()),
		}
	}
	return s.ka
}

var (
	ErrSessionState  = errors.New("bad DSO state")
	ErrSessionClosed = fmt.Errorf("%w: closed", ErrSessionState)
)
