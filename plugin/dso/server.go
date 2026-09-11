package dso

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dso/internal/dsomessage"
	"github.com/coredns/coredns/plugin/dso/internal/dsosession"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

var ErrServerClosed = errors.New("server closed")

type (
	Server struct {
		Config   *Config
		Upstream *dnsserver.Server

		shutdown     atomic.Bool
		shutdownCtx  context.Context
		shutdownFunc context.CancelCauseFunc

		listeners      map[net.Listener]struct{}
		listenersGroup sync.WaitGroup

		conns      map[net.Conn]*connHandler
		connsGroup sync.WaitGroup

		mu sync.Mutex
	}

	connState struct {
		sesh *dsosession.Session
		push *dsosession.Push
		ka   dsomessage.KeepAlive

		config   *Config
		upstream *dnsserver.Server

		label string
	}

	// connHandler is persistant handler of entire client connection.
	connHandler struct {
		*connState

		startAt  time.Time
		activeAt atomic.Int64
		aliveAt  atomic.Int64
	}

	// dnsMsgHandler is transient handler of single DNS message.
	dnsMsgHandler struct {
		*connState

		tsigStatus error
		tsigSecret string
		tsigMAC    string
	}

	// dsoMsgHandler is transient handler of single DSO message.
	dsoMsgHandler struct {
		*connState

		parser    dsomessage.Parser
		msgHeader dsomessage.MsgHeader
		tlvHeader dsomessage.TLVHeader
	}
)

// Serve serves DNS and DSO over accepted connections.
func (s *Server) Serve(ln net.Listener) error {
	if !s.trackListener(ln, true) {
		return ErrServerClosed
	}
	defer s.trackListener(ln, false)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.shutdown.Load() {
				return ErrServerClosed
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return err
		}
		if !s.trackConn(conn, true) {
			conn.Close()
			return ErrServerClosed
		}
	}
}

// ServeTLS serves DNS and DSO via TLS over accepted connections.
func (s *Server) ServeTLS(ln net.Listener) error {
	return s.Serve(tls.NewListener(ln, s.Config.TLSConfig.Clone()))
}

// RefreshPushSubscriptions schedules
func (s *Server) RefreshPushSubscriptions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, conn := range s.conns {
		if conn.push != nil {
			conn.push.Refresh()
		}
	}
}

// Shutdown tears down listeners and connections.
func (s *Server) Shutdown(ctx context.Context, reconnectInterval time.Duration) error {
	if !s.shutdown.CompareAndSwap(false, true) {
		s.listenersGroup.Wait()
		s.connsGroup.Wait()
		return nil
	}

	if s.listeners == nil {
		return nil
	}

	s.shutdownFunc(&shutdownError{dns.RcodeSuccess, reconnectInterval})

	s.mu.Lock()
	for l := range s.listeners {
		l.Close()
	}
	s.mu.Unlock()
	s.listenersGroup.Wait()

	// All [Server.Serve] calls returned and no more connections are accepted.

	connsDoneC := make(chan struct{})
	go func() {
		s.connsGroup.Wait()
		close(connsDoneC)
	}()
	select {
	case <-connsDoneC:
	case <-ctx.Done():
	}

	s.mu.Lock()
	for _, h := range s.conns {
		h.sesh.Abort()
	}
	s.mu.Unlock()
	s.connsGroup.Wait()

	return nil
}

func (s *Server) trackListener(ln net.Listener, add bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listeners == nil {
		s.listeners = make(map[net.Listener]struct{})
		s.conns = make(map[net.Conn]*connHandler)
		s.shutdownCtx, s.shutdownFunc = context.WithCancelCause(context.Background())
	}

	if add {
		if s.shutdown.Load() {
			ln.Close()
			return false
		}
		s.listeners[ln] = struct{}{}
		s.listenersGroup.Add(1)
	} else {
		delete(s.listeners, ln)
		s.listenersGroup.Done()
	}
	return true
}

func (s *Server) trackConn(conn net.Conn, add bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if add {
		if s.shutdown.Load() {
			return false
		}
		h := newConnHandler(s, conn)
		s.conns[conn] = h
		s.connsGroup.Go(func() {
			defer s.trackConn(conn, false)
			h.handle(s.shutdownCtx)
		})
	} else {
		delete(s.conns, conn)
	}
	return true
}

func newConnHandler(server *Server, conn net.Conn) (h *connHandler) {
	h = &connHandler{
		connState: &connState{
			sesh: dsosession.New(conn),
			ka: dsomessage.KeepAlive{
				InactivityTimeout: uint32(server.Config.InactivityTimeout.Milliseconds()),
				KeepAliveInterval: uint32(server.Config.KeepAliveInterval.Milliseconds()),
			},

			config:   server.Config,
			upstream: server.Upstream,

			label: server.Upstream.Addr,
		},
		startAt: time.Now(),
	}
	return h
}

func (h *connHandler) handle(ctx context.Context) {
	connCtx, cancelFunc := context.WithCancelCause(ctx)

	// Plain DNS is only answered to satisfy RFC 8765 (Push) requirement.
	_, isTLS := h.sesh.Conn.(*tls.Conn)
	if cfg := h.config.Push; isTLS && cfg != nil {
		h.push = dsosession.NewPush(h.config.Push.Classes, h.config.Push.Types)
		go func() {
			err := h.push.Serve(connCtx, h, h, h.config.Push.DebounceDelay, h.config.Push.RefreshInterval)
			cancelFunc(err)
		}()
	}

	go func() {
		err := h.serve(connCtx)
		cancelFunc(err)
	}()

	<-connCtx.Done()
	err := context.Cause(connCtx)

	if shutdownErr, ok := errors.AsType[*shutdownError](err); ok {
		retryDelay := uint32(shutdownErr.reconnectInterval.Milliseconds()) // #nosec G115
		err = h.closeNotify(dns.RcodeSuccess, retryDelay)
	}

	switch {
	case err == nil:
		fallthrough
	case errors.Is(err, dsosession.ErrStateClosed):
		io.Copy(io.Discard, h.sesh.Conn)
		h.sesh.Close()

	case errors.Is(err, net.ErrClosed):
		fallthrough
	case errors.Is(err, syscall.EPIPE):
		fallthrough
	case errors.Is(err, io.EOF):
		h.sesh.Close()

	default:
		h.sesh.Abort()
	}
}

func (h *connHandler) serve(ctx context.Context) error {
	defer func() {
		if h.push != nil {
			for sub := range h.push.Subscriptions() {
				rrtype := dns.Type(sub.RRType).String()
				pushSubscriptionEntries.WithLabelValues(h.label, rrtype).Dec()
			}
		}
		sessionDuration.WithLabelValues(h.label).Observe(time.Since(h.startAt).Seconds())
	}()

	dnsTimeout := h.upstream.IdleTimeout // only first read is subject to idling
	msg, err := h.read(h.deadline(dnsTimeout))
	if err != nil {
		return err
	}

	h.tickAlive(true)

	dnsTimeout = h.upstream.ReadTimeout
	for {
		if err = ctx.Err(); err != nil {
			return err
		}

		if rawMsg(msg).opcode() == dns.OpcodeStateful {
			err = (&dsoMsgHandler{connState: h.connState}).handle(h, msg)
		} else {
			err = (&dnsMsgHandler{connState: h.connState}).handle(h, h, msg)
		}
		if err != nil {
			return err
		}

		if err = ctx.Err(); err != nil {
			return err
		}

		msg, err = h.read(h.deadline(dnsTimeout))
		if err != nil {
			return err
		}
	}
}

// tickAlive ticks alive and activity timestamps.
func (h *connHandler) tickAlive(active bool) {
	t := time.Since(h.startAt)
	h.aliveAt.Store(int64(t))
	if active {
		h.activeAt.Store(int64(t))
	}
}

// deadline calculates deadline for next read operation.
//
// Timeout depends on whether there is active DSO session.
func (h *connHandler) deadline(dnsTimeout time.Duration) time.Time {
	if h.sesh.State() != dsosession.StateEstablished {
		return time.Now().Add(dnsTimeout)
	}

	// RFC 8490, Section 6.4.1: An operation being active on a DSO Session includes
	// ... an active long-lived operation
	//
	// RFC 8490, Section 6.4.2: A server will forcibly abort an idle client session after five
	// seconds or twice the inactivity timeout value, whichever is greater.
	//
	// RFC 8490, Section 6.4.2: In the case of a zero inactivity timeout value,
	// this means that if a client fails to close an idle client session, then the server
	// will forcibly abort the idle session after five seconds.
	//
	// RFC 8490, Section 6.4.2: An inactivity timeout of 0xFFFFFFFF represents "infinity"
	// and informs the client that it may keep an idle connection open as long as it wishes.
	var activeDeadline time.Duration
	if h.push == nil || !h.push.IsActive() {
		if t := h.ka.InactivityTimeout; t != dsomessage.InactivityTimeoutNever {
			activeDeadline = time.Duration(h.activeAt.Load()) + max(5*time.Second, 2*(time.Duration(t)*time.Millisecond))
		}
	}

	// RFC 8490, Section 6.5.1: If, at any time during the life of the DSO Session,
	// twice the keepalive interval value (i.e., 30 seconds by default) elapses without
	// any DNS messages being sent or received on a DSO Session, the server SHOULD consider
	// the client delinquent and SHOULD forcibly abort the DSO Session.
	//
	// RFC 8490, Section 6.5.2: the server MUST NOT send a DSO Keepalive message ...
	// with a keepalive interval value less than ten seconds
	//
	// RFC 8490, Section 6.5.2: A keepalive interval value of 0xFFFFFFFF represents
	// "infinity" and informs the client that it should generate no DSO keepalive traffic.
	var aliveDeadline time.Duration
	if t := h.ka.KeepAliveInterval; t != dsomessage.KeepAliveIntervalNever {
		aliveDeadline = time.Duration(h.aliveAt.Load()) + 2*max(dsomessage.KeepAliveIntervalMin*time.Millisecond, time.Duration(t)*time.Millisecond)
	}

	switch {
	case activeDeadline > 0 && aliveDeadline > 0:
		return h.startAt.Add(min(activeDeadline, aliveDeadline))
	case activeDeadline > 0:
		return h.startAt.Add(activeDeadline)
	case aliveDeadline > 0:
		return h.startAt.Add(aliveDeadline)
	default:
		return time.Time{}
	}
}

func (h *connHandler) read(deadline time.Time) (msg []byte, err error) {
	msg, err = h.sesh.ReadMsg(deadline)
	if err != nil {
		return nil, err
	}

	// RFC 8490, Section 6.3: At both servers and clients, the generation or reception
	// of any complete DNS message ... resets both timers for that DSO Session, with
	// the one exception being that a DSO Keepalive message resets only the keepalive timer,
	// not the inactivity timeout timer.
	raw := rawMsg(msg)
	if raw.opcode() == dns.OpcodeStateful {
		if h.config.Log != nil {
			log.Info(formatLog(h.sesh.Conn, dsomessage.OriginClient, msg))
		}
		h.tickAlive(!raw.isKeepAliveMsg())
	} else {
		h.tickAlive(true)
	}

	return msg, nil
}

// closeNotify asks client to terminate connection.
func (h *connHandler) closeNotify(rcode uint8, retryDelay uint32) (err error) {
	const BufLen = dsomessage.LengthPrefixLen + dsomessage.MsgHeaderLen + dsomessage.TLVHeaderLen + dsomessage.RetryDelayLen
	var buf [BufLen]byte
	b := dsomessage.NewBuilder(buf[:]).
		EnableLengthPrefix().
		SetHeader(dsomessage.MsgHeader{ID: 0, Response: false, Rcode: rcode})
	b.WriteRetryDelay(&dsomessage.RetryDelay{RetryDelay: retryDelay})
	msg, _ := b.Message()
	_, err = h.Write(msg)
	return err
}

func (h *connHandler) Write(msg []byte) (n int, err error) {
	raw := rawMsg(msg[dsomessage.LengthPrefixLen:])
	if raw.opcode() == dns.OpcodeStateful {
		if h.config.Log != nil {
			log.Info(formatLog(h.sesh.Conn, dsomessage.OriginServer, msg[dsomessage.LengthPrefixLen:]))
		}
		defer h.tickAlive(!raw.isKeepAliveMsg()) // see [connHandler.read]
	} else {
		defer h.tickAlive(true) // see [connHandler.read]
	}
	return h.sesh.Write(msg)
}

func (h *dnsMsgHandler) handle(writer io.Writer, upstream *connHandler, msg []byte) (err error) {
	query, err := h.unpackMsg(msg)
	if err != nil {
		// Header is guaranteed to exist but everything else is as good as malformed.
		answer := &dns.Msg{
			MsgHdr: dns.MsgHdr{
				Id:       rawMsg(msg).id(),
				Response: true,
				Opcode:   rawMsg(msg).opcode(),
				Rcode:    dns.RcodeFormatError,
			},
		}
		if answer.Opcode == dns.OpcodeQuery {
			answer.RecursionDesired = rawMsg(msg).recursionDesired()
			answer.CheckingDisabled = rawMsg(msg).checkingDisabled()
		}
		return h.writeMsg(writer, answer, msg)
	}

	rcode, err := h.verifyMsg(msg, query)
	if err != nil {
		return err
	}
	if rcode != dns.RcodeSuccess {
		// Query should not be forwarded upstream, but error is not fatal.
		answer := new(dns.Msg).SetRcode(query, rcode)
		state := request.Request{Req: query, W: nil}
		state.SizeAndDo(answer)
		if h.config.TsigSecret != nil {
			if t := query.IsTsig(); t != nil {
				tsigRR := &dns.TSIG{
					Hdr: dns.RR_Header{
						Name:   t.Hdr.Name,
						Rrtype: dns.TypeTSIG,
						Class:  dns.ClassANY,
					},
					Algorithm: t.Algorithm,
					OrigId:    query.Id,

					// Used by [dns.TsigGenerate].
					MACSize: t.MACSize,
					MAC:     t.MAC,
				}
				if h.tsigStatus != nil {
					// TSIG verification errors take precedence.
					answer.Rcode = dns.RcodeNotAuth
					switch h.tsigStatus {
					case dns.ErrTime:
						tsigRR.Error = dns.RcodeBadTime
						// See [tsig.restoreTsigWriter.WriteMsg].
						tsigRR.TimeSigned = t.TimeSigned
						b := make([]byte, 8)
						binary.BigEndian.PutUint64(b, uint64(time.Now().Unix()))
						tsigRR.OtherData = hex.EncodeToString(b[2:])
						tsigRR.OtherLen = 6
					case dns.ErrSecret:
						tsigRR.Error = dns.RcodeBadKey
					default:
						tsigRR.Error = dns.RcodeBadSig
					}
				}
				answer.Extra = append(answer.Extra, tsigRR)
			}
		}
		return h.writeMsg(writer, answer, msg)
	}

	answer := upstream.lookupDNS(context.Background(), query, h.tsigStatus)
	return h.writeMsg(writer, answer, msg)
}

// packMsg packs message and signs it, if TSIG is present.
func (h *dnsMsgHandler) packMsg(m *dns.Msg, buf []byte) (msg []byte, err error) {
	if h.config.TsigSecret != nil {
		if t := m.IsTsig(); t != nil {
			buf, h.tsigMAC, err = dns.TsigGenerate(m, h.tsigSecret, h.tsigMAC, false)
		} else {
			buf, err = m.PackBuffer(buf)
		}
	} else {
		buf, err = m.PackBuffer(buf)
	}
	if err != nil {
		return nil, err
	}

	msgLen := len(buf)
	if cap(buf)-msgLen < 2 {
		buf1 := make([]byte, 2+msgLen)
		copy(buf1[2:], buf)
		buf = buf1
	} else {
		buf = buf[:msgLen+2]
		copy(buf[2:], buf)
	}
	binary.BigEndian.PutUint16(buf, uint16(msgLen))
	return buf, nil
}

// unpackMsg unpacks message and verifies attached TSIG, if any.
func (h *dnsMsgHandler) unpackMsg(msg []byte) (query *dns.Msg, err error) {
	query = new(dns.Msg)
	if err := query.Unpack(msg); err != nil {
		return nil, err
	}
	if h.config.TsigSecret != nil {
		if t := query.IsTsig(); t != nil {
			var ok bool
			if h.tsigSecret, ok = h.config.TsigSecret[t.Hdr.Name]; ok {
				h.tsigStatus = dns.TsigVerify(msg, h.tsigSecret, "", false)
				h.tsigMAC = t.MAC
			} else {
				h.tsigStatus = dns.ErrSecret
			}
		}
	}
	return query, nil
}

// verifyMsg checks if query should be forwarded to upstream.
//
// Uses strictier of custom and [dns.DefaultMsgAcceptFunc] checks.
func (h *dnsMsgHandler) verifyMsg(msg []byte, query *dns.Msg) (rcode int, err error) {
	if query.Opcode != dns.OpcodeQuery {
		return dns.RcodeNotImplemented, nil
	}
	if query.Response {
		return dns.RcodeRefused, errUnexpected
	}
	if len(query.Question) != 1 {
		return dns.RcodeFormatError, nil
	}
	if len(query.Answer) != 0 {
		return dns.RcodeFormatError, nil
	}
	if len(query.Ns) != 0 {
		return dns.RcodeFormatError, nil
	}
	if len(query.Extra) > 2 {
		return dns.RcodeFormatError, nil
	}

	switch dns.DefaultMsgAcceptFunc(rawMsg(msg).header()) {
	case dns.MsgAccept:

	case dns.MsgRejectNotImplemented:
		return dns.RcodeNotImplemented, nil
	case dns.MsgReject:
		return dns.RcodeFormatError, nil
	default:
		return dns.RcodeRefused, errUnexpected
	}

	// See [dsosession.Push.CanAdd].
	switch query.Question[0].Qtype {
	case dns.TypeOPT, dns.TypeTSIG, dns.TypeRRSIG:
		fallthrough
	case dns.TypeNone, dns.TypeNXNAME, dns.TypeIXFR, dns.TypeAXFR, dns.TypeMAILB, dns.TypeMAILA:
		return dns.RcodeFormatError, nil
	}

	// Only answer to satisfy RFC 8765, Section 3 requirement
	if h.push == nil {
		return dns.RcodeRefused, nil
	}

	// RFC 8490, Section 7.1.2: Once a DSO Session has been established, if ... receives
	// a DNS message ... that contains an edns-tcp-keepalive EDNS(0) Option, this is a fatal error
	// and the receiver ... MUST forcibly abort the connection immediately.
	if h.sesh.State() == dsosession.StateEstablished {
		if edns0 := query.IsEdns0(); edns0 != nil {
			for _, opt := range edns0.Option {
				if opt.Option() == dns.EDNS0TCPKEEPALIVE {
					return dns.RcodeFormatError, errEDNS0KeepAlive
				}
			}
		}
	}

	// Only answer queries for authorized zones.
	if !slices.ContainsFunc(h.config.Push.Zones, func(z string) bool { return dns.IsSubDomain(z, query.Question[0].Name) }) {
		return dns.RcodeNotAuth, nil
	}

	return dns.RcodeSuccess, nil
}

func (h *dnsMsgHandler) writeMsg(writer io.Writer, m *dns.Msg, buf []byte) (err error) {
	msg, err := h.packMsg(m, buf)
	if err != nil {
		return err
	}
	_, err = writer.Write(msg)
	return err
}

func (h *dsoMsgHandler) handle(writer io.Writer, msg []byte) (err error) {
	defer func() {
		// RFC 8490, Section 6.6.1.1: At the instant a server chooses to initiate
		// a DSO Retry Delay message, there may be DNS requests already in flight
		// from client to server on this DSO Session, which will arrive at the server
		// after its DSO Retry Delay message has been sent. The server MUST silently
		// ignore such incoming requests and MUST NOT generate any response
		// messages for them.
		if errors.Is(err, dsosession.ErrStateClosed) {
			err = nil
		}
		if errors.Is(err, dsosession.ErrState) {
			err = errUnexpected
		}
	}()

	h.msgHeader, err = h.parser.Start(msg, dsomessage.OriginClient)
	if err != nil {
		return err
	}

	// RFC 8490, Section 5.2.2: If a client or server receives a response (QR=1)
	// where the MESSAGE ID is zero, or is any other value that does not match
	// the MESSAGE ID of any of its outstanding operations, this is a fatal error
	// and the recipient MUST forcibly abort the connection immediately.
	if !h.msgHeader.IsRequest() && !h.msgHeader.IsUnidirectional() {
		return errUnexpected
	}

	h.tlvHeader, err = h.parser.TLVHeader()
	if err != nil {
		return err
	}

	switch h.tlvHeader.Type {
	case dsomessage.TypeKeepAlive:
		return h.handleKeepAlive(writer, msg)
	case dsomessage.TypeSubscribe:
		return h.handleSubscribe(writer, msg)
	case dsomessage.TypeUnsubscribe:
		return h.handleUnsubscribe()
	case dsomessage.TypeReconfirm:
		return h.handleReconfirm()
	default:
		return h.handleUnexpected(writer, msg)
	}
}

func (h *dsoMsgHandler) handleKeepAlive(writer io.Writer, msg []byte) (err error) {
	ka, err := parseTLV(dsomessage.UsageCP, h.parser.TLVUsage(), h.parser.KeepAlive)
	if err != nil {
		return err
	}

	// Accept reduced timeout and keepalive as it helps server shed connection sooner.
	// Note server merely agrees to receive KA more often.
	h.ka.InactivityTimeout = min(uint32(h.config.InactivityTimeout.Milliseconds()), ka.InactivityTimeout)
	h.ka.KeepAliveInterval = max(min(uint32(h.config.KeepAliveInterval.Milliseconds()), ka.KeepAliveInterval), dsomessage.KeepAliveIntervalMin)

	_, err = writer.Write(h.buildKeepAlive(h.msgHeader.ID, msg))
	return err
}

func (h *dsoMsgHandler) handleSubscribe(writer io.Writer, msg []byte) (err error) {
	// RFC 8765, Section 6.2.2: For RCODE = 5 (REFUSED), which occurs on a server that
	// implements DNS Push Notifications but is currently configured to disallow
	// DNS Push Notifications
	if h.push == nil {
		_, err = writer.Write(h.buildError(dns.RcodeRefused, msg))
		return err
	}

	tlv, err := parseTLV(dsomessage.UsageCP, h.parser.TLVUsage(), h.parser.Subscribe)
	if err != nil {
		return err
	}

	rrtype := dns.Type(tlv.RRType).String()
	pushSubscriptionRequests.WithLabelValues(h.label, rrtype).Inc()

	// RFC 8765, Section 6.2.2: For RCODE = 9 (NOTAUTH), which occurs on a server
	// that implements DNS Push Notifications but is not configured to be authoritative
	// for the requested name
	if !plugin.Zones(h.config.Push.Zones).Contains(tlv.Name) {
		_, err = writer.Write(h.buildError(dns.RcodeNotAuth, msg))
		return err
	}

	sub, rcode, err := h.push.CanAdd(h.msgHeader.ID, tlv)
	if err != nil {
		return fmt.Errorf("%w - %w", errMsg, err)
	}
	if rcode != dns.RcodeSuccess {
		_, err = writer.Write(h.buildError(rcode, msg))
		return err
	}

	// Confirm subscription before updates.
	builder := h.newBuilder(0, msg).
		SetHeader(dsomessage.MsgHeader{
			ID:       h.msgHeader.ID,
			Response: true,
			Rcode:    dns.RcodeSuccess,
		})
	msg, _ = builder.Message()
	_, err = writer.Write(msg)
	if err != nil {
		return err
	}

	pushSubscriptionHits.WithLabelValues(h.label, rrtype).Inc()
	pushSubscriptionEntries.WithLabelValues(h.label, rrtype).Inc()

	h.push.Add(h.msgHeader.ID, sub)
	return nil
}

func (h *dsoMsgHandler) handleUnsubscribe() (err error) {
	if h.push == nil || !h.push.IsActive() {
		return nil
	}

	tlv, err := parseTLV(dsomessage.UsageCU, h.parser.TLVUsage(), h.parser.Unsubscribe)
	if err != nil {
		return err
	}

	sub, ok := h.push.Remove(tlv)
	if ok {
		rrtype := dns.Type(sub.RRType).String()
		pushSubscriptionEntries.WithLabelValues(h.label, rrtype).Inc()
	}
	return nil
}

func (h *dsoMsgHandler) handleReconfirm() (err error) {
	if h.push == nil || !h.push.IsActive() {
		return nil
	}

	tlv, err := parseTLV(dsomessage.UsageCU, h.parser.TLVUsage(), h.parser.Reconfirm)
	if err != nil {
		return err
	}

	h.push.Reconfirm(tlv)
	return nil
}

func (h *dsoMsgHandler) handleUnexpected(writer io.Writer, msg []byte) (err error) {
	// RFC 8490, Section 5.4.1: Whether a message is a DSO ... request or ... unidirectional
	// message is determined only by the specification for the Primary TLV ... A responder
	// that receives ... malformed message MUST ... forcibly abort the connection immediately.
	//
	// RFC 8490, Section 5.4.5: If a DSO unidirectional message is received containing
	// ... an unrecognized Primary TLV ... then this is a fatal error and the recipient
	// MUST forcibly abort the connection immediately.
	//
	// RFC 8490, Section 5.4.5: If a DSO request message is received containing
	// an unrecognized Primary TLV ... then the receiver MUST send an error response
	// with a matching MESSAGE ID, and RCODE DSOTYPENI.
	switch {
	case h.msgHeader.IsUnidirectional():
		fallthrough
	case h.tlvHeader.Type == dsomessage.TypeRetryDelay:
		fallthrough
	case h.tlvHeader.Type == dsomessage.TypeEncryptionPadding:
		fallthrough
	case h.tlvHeader.Type == dsomessage.TypePush:
		return errUnexpected
	default:
		_, err = writer.Write(h.buildError(dns.RcodeStatefulTypeNotImplemented, msg))
		return err
	}
}

func (h *dsoMsgHandler) newBuilder(dsoLen int, buf []byte) (b *dsomessage.Builder) {
	b = dsomessage.NewBuilder(buf).EnableLengthPrefix()
	// RFC 8490, Section 7.3: It is only applicable when the DSO Transport layer uses
	// encryption such as TLS.
	_, isTLS := h.sesh.Conn.(*tls.Conn)
	if isTLS && h.parser.IsPadded() {
		b.EnablePadding(dsomessage.TLSBlockLen)
	}
	b.Grow(dsoLen)
	return b
}

func (h *dsoMsgHandler) buildKeepAlive(id uint16, buf []byte) (msg []byte) {
	b := h.newBuilder(dsomessage.TLVHeaderLen+dsomessage.KeepAliveLen, buf).
		SetHeader(dsomessage.MsgHeader{
			ID:       id,
			Response: id != 0, // server either responds to client or writes unidirectional
			Rcode:    dns.RcodeSuccess,
		})
	b.WriteKeepAlive(&h.ka)
	msg, _ = b.Message()
	return msg
}

func (h *dsoMsgHandler) buildError(rcode uint8, buf []byte) (msg []byte) {
	b := h.newBuilder(0, buf).SetHeader(dsomessage.MsgHeader{
		ID:       h.msgHeader.ID,
		Response: true,
		Rcode:    rcode,
	})
	msg, _ = b.Message()
	return msg
}

type lookupResponse struct {
	Msg *dns.Msg

	local      net.Addr
	remote     net.Addr
	tsigStatus error
}

func (w *lookupResponse) LocalAddr() net.Addr       { return w.local }
func (w *lookupResponse) RemoteAddr() net.Addr      { return w.remote }
func (w *lookupResponse) WriteMsg(m *dns.Msg) error { w.Msg = m; return nil }
func (w *lookupResponse) Write(b []byte) (int, error) {
	w.Msg = new(dns.Msg)
	return len(b), w.Msg.Unpack(b)
}
func (w *lookupResponse) Close() error          { return nil }
func (w *lookupResponse) TsigStatus() error     { return w.tsigStatus }
func (w *lookupResponse) TsigTimersOnly(_ bool) {}
func (w *lookupResponse) Hijack()               {}

func (h *connHandler) lookupDNS(ctx context.Context, query *dns.Msg, tsigStatus error) *dns.Msg {
	dnsCtx := context.WithValue(ctx, dnsserver.Key{}, h.upstream)
	dnsCtx = context.WithValue(dnsCtx, dnsserver.LoopKey{}, 0)
	w := &lookupResponse{local: h.sesh.Conn.LocalAddr(), remote: h.sesh.Conn.RemoteAddr(), tsigStatus: tsigStatus}
	h.upstream.ServeDNS(dnsCtx, w, query)
	if w.Msg == nil {
		return new(dns.Msg).SetRcode(query, dns.RcodeServerFailure)
	}
	return w.Msg
}

// Lookup implements [dsosession.PushLookuper.LookupPushSubscription].
func (h *connHandler) LookupPushSubscription(ctx context.Context, tlv dsomessage.Subscribe) ([]dns.RR, bool) {
	query := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:               dns.Id(),
			RecursionDesired: true,
		},
		Question: []dns.Question{
			{
				Name:   tlv.Name,
				Qtype:  tlv.RRType,
				Qclass: tlv.Class,
			},
		},
	}
	answer := h.lookupDNS(ctx, query, nil)
	if answer == nil {
		return nil, false
	}

	if answer.Opcode != dns.OpcodeQuery || !answer.Response {
		return nil, false
	}

	tp, _ := response.Typify(answer, time.Now())
	switch tp {
	case response.NoError:
	case response.NameError:
		fallthrough
	case response.NoData:
		return nil, true
	default:
		return nil, false
	}

	// Push wants either single CNAME or RRset with matching TYPE.
	var (
		rrs   = slices.Clone(answer.Answer)
		cname dns.RR
		i     int
	)
FilterRRset:
	for j := range rrs {
		h := rrs[j].Header()
		if h.Class != tlv.Class {
			continue
		}
		switch h.Rrtype {
		case tlv.RRType:
			if cname == nil && strings.EqualFold(h.Name, tlv.Name) {
				rrs[i] = rrs[j]
				i++
			}
		case dns.TypeDNAME: // DNAME redirect occludes everthing else
			if dns.IsSubDomain(h.Name, tlv.Name) && !strings.EqualFold(h.Name, tlv.Name) { // strict subdomain
				labels := dns.SplitDomainName(tlv.Name)
				labels = append(labels[0:len(labels)-dns.CountLabel(h.Name)], dns.SplitDomainName(rrs[j].(*dns.DNAME).Target)...)
				cname = &dns.CNAME{
					Hdr: dns.RR_Header{
						Name:   tlv.Name,
						Rrtype: dns.TypeCNAME,
						Class:  h.Class,
						Ttl:    h.Ttl,
					},
					Target: dnsutil.Join(labels...),
				}
				break FilterRRset
			}
		case dns.TypeCNAME: // only use CNAME if DNAME is not provided
			if cname == nil && strings.EqualFold(h.Name, tlv.Name) {
				cname = rrs[j]
			}
		}
	}
	if cname != nil {
		return []dns.RR{cname}, true
	}
	clear(rrs[i:])
	return rrs[:i], true
}

type shutdownError struct {
	rcode             int
	reconnectInterval time.Duration
}

func (e *shutdownError) Error() string {
	return "dso.Server: shutdown"
}

type ptrTLV[T any] interface {
	dsomessage.TLV
	*T
}

// parseTLV checks usage, unpacks and verifies TLV.
func parseTLV[P ptrTLV[T], T any](expected, actual dsomessage.Usage, parseFunc func() (T, error)) (tlv T, err error) {
	if expected != actual {
		return tlv, errUnexpected
	}
	tlv, err = parseFunc()
	if err != nil {
		return tlv, err
	}
	return tlv, P(&tlv).Verify(actual)
}

type rawMsg []byte

func (m rawMsg) id() uint16 {
	return binary.BigEndian.Uint16(m)
}

func (m rawMsg) opcode() int {
	return int(m[2]&0b0_1111_0_0_0) >> 3
}

func (m rawMsg) recursionDesired() bool {
	return (m[2] & 0b0_0000_0_0_1) != 0
}

func (m rawMsg) checkingDisabled() bool {
	return (m[3] & 0b0_0_0_1_0000) != 0
}

func (m rawMsg) header() dns.Header {
	return dns.Header{
		Id:      m.id(),
		Bits:    binary.BigEndian.Uint16(m[2:]),
		Qdcount: binary.BigEndian.Uint16(m[4:]),
		Ancount: binary.BigEndian.Uint16(m[6:]),
		Nscount: binary.BigEndian.Uint16(m[8:]),
		Arcount: binary.BigEndian.Uint16(m[10:]),
	}
}

// isKeepAliveMsg checks whether DSO msg's primary TLV is KeepAlive.
func (m rawMsg) isKeepAliveMsg() bool {
	return len(m) >= dsomessage.MsgHeaderLen+dsomessage.TLVHeaderLen+dsomessage.KeepAliveLen && dsomessage.Type(binary.BigEndian.Uint16(m[dsomessage.MsgHeaderLen:])) == dsomessage.TypeKeepAlive
}

var (
	errMsg            = fmt.Errorf("bad message")
	errEDNS0KeepAlive = fmt.Errorf("%w - EDNS0 keepalive during active DSO session", errMsg)
	errUnexpected     = fmt.Errorf("%w - unexpected", errMsg)
)
