package dso

import (
	"context"
	ctls "crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dso/internal/dsomessage"
	"github.com/coredns/coredns/plugin/dso/internal/dsosession"

	"github.com/google/go-cmp/cmp"
	"github.com/miekg/dns"
)

// localhostCert is a PEM-encoded TLS cert with SAN IPs
// "127.0.0.1" and "[::1]", expiring at Jan 29 16:00:00 2084 GMT.
// generated from src/crypto/tls:
// go run generate_cert.go --rsa-bits 2048 --host "127.0.0.1,::1,example.com,*.example.com,.test,*.test" --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h
var localhostCert = []byte(`-----BEGIN CERTIFICATE-----
MIIDVzCCAj+gAwIBAgIQfWtS/atiL9btzVfpJ5yj8zANBgkqhkiG9w0BAQsFADAS
MRAwDgYDVQQKEwdBY21lIENvMCAXDTcwMDEwMTAwMDAwMFoYDzIwODQwMTI5MTYw
MDAwWjASMRAwDgYDVQQKEwdBY21lIENvMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEAqET95XolstZEkrE8VWvD0ApBlYVKE28Fgk3ppat8gL47y4edDizF
pniTvBWVewIrzgh0VLZlcCqfqxL9cHzZOln4cHSctHNO2GauDR7QL5m0LOxY4OnB
E+jVG4IybXpU5PDJEdgqeV1fqsoaW2Dp+D12NYJGd3hibdO/kXhqd9F89Y+/D6eo
CPJfnQsGD6z07Cc5uWfNRSLE3r/ChZYEUxukb9lULGAmeckfHCUnKQhGZbkvWJQf
a/DDwEVgevrGL6G0Vz0dxACO8kXwSKxx/PU+Fx9pn/D5OLgCJwNFAHIwjIAkaavM
eEiy3q0OOvd1tM3zTpkqcudxhpZD43byWQIDAQABo4GmMIGjMA4GA1UdDwEB/wQE
AwICpDATBgNVHSUEDDAKBggrBgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MB0GA1Ud
DgQWBBRcZcBGyf8vwhCegpiVndG+W/4E0jBMBgNVHREERTBDggtleGFtcGxlLmNv
bYINKi5leGFtcGxlLmNvbYIFLnRlc3SCBioudGVzdIcEfwAAAYcQAAAAAAAAAAAA
AAAAAAAAATANBgkqhkiG9w0BAQsFAAOCAQEAAzrSVzptvOEbkRF6KtWzB9psU7Uz
CMcM2Tn3nlBf3C/+YtR2wdHFELQzKvMfayvg9fcuX9ZQuvTw0dHpTJmF26PY5X7k
2se6EUSlBquC/LQmZknZTZYX6aoR/WseYZmejhPpRfKIN0oGta1cAQd9f84guXCC
6p2BoUgK+RPPbP/CFxAWWzMk/q+zs7/b0N+fDRBCTfMtfHs3bjrANq1aoJyZPMiu
1u3rBRLqCFSk0zaz3IE9rPpZnperf/+3UbhYszlVvOANAj4kbEFWpUGUTJIPE08A
JnCrXLQbvBz/l7kpV0tUGPmLQQSKF9xFEfjYTGqXi/NPOPOpUtPeahI3lA==
-----END CERTIFICATE-----`)

// localhostKey is the private key for LocalhostCert.
var localhostKey = []byte(`-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCoRP3leiWy1kSS
sTxVa8PQCkGVhUoTbwWCTemlq3yAvjvLh50OLMWmeJO8FZV7AivOCHRUtmVwKp+r
Ev1wfNk6WfhwdJy0c07YZq4NHtAvmbQs7Fjg6cET6NUbgjJtelTk8MkR2Cp5XV+q
yhpbYOn4PXY1gkZ3eGJt07+ReGp30Xz1j78Pp6gI8l+dCwYPrPTsJzm5Z81FIsTe
v8KFlgRTG6Rv2VQsYCZ5yR8cJScpCEZluS9YlB9r8MPARWB6+sYvobRXPR3EAI7y
RfBIrHH89T4XH2mf8Pk4uAInA0UAcjCMgCRpq8x4SLLerQ4693W0zfNOmSpy53GG
lkPjdvJZAgMBAAECggEAE+DeoQLpPDN4M1+GZ02ipXY+DDS3AmJjDg3ilG8/TJLs
oR0wDnIh7AoiOHMmTxCmbcxtjb+5w7f4AoTs9XfSTm89hxoc6cEWJ0DDaV+4AWo7
hkkU24Z8iDvBHThSzmsdZ1RotIJ1y2jz+csECm0sXBFKtdniNY/pCjIcgfTDQyHb
x+llGxCoeH2uVF7kHeduOn4CQDBF1CojBtYTzVY9/PI1hpneE0Wi03jtQUest03Y
Tr+7AltqGNwyARjzHLi3Be1vT/pvRoR+f9DeaS0zgg9PSTJUqfek6YQOxsjqGVDR
opGKgHC2wBzy2ul5YfnWrWkF7EblEWn3Dyto/8G4/wKBgQDabZdAcCOdri7MQ82q
JuIrEouoYpTtjhdp9+UNri8a49ap3ffsUNZyscDK8QibIpooQyI8xRH1mmAQo+p2
3035EmO19HMeP8o2jJBjzCSo6yWwVx0V2t3Pq8dbs5lza1ntBznv3heQUlGZHTSt
ROJNTPhnAPy7OFZ7RiDtnyyKWwKBgQDFNq8VLZAPAwqNLh1ft/rpQ9WYcRaPCtbl
RBiZzVIuETf35z+jFakOapSf3CRYWBsaRjuxvK2AwXgXnYALOvEKazZhcrJNvYt2
GLhYql9NKfGWwp4PZvWxmXkRzFqNO8l/VxNetckn4RaeZ0ArGrvxCjf2aXr+YcaK
TIXXmmiMWwKBgBjo+qDcqRModCnTabcH7C8hVFAFvhpBZCYvoS2oObMFXMvOhqGq
rmoyH1yFlIessIv67AKmLuAllOMQ7oJUAR5wnHJ5yE8g0zzZVvYqp9ujxY6QwL5n
UXiHjJrGpq9lBMJlWpQibemFmcyuaf2Ap5ZNOt70W942FJbGbqbqyjeVAoGBALxd
1+tNgqykBf8FTe8wJouJTEn3sklcXBfN7AVzlIwFzESP7zuRI9FuQZlTRq/PL8vv
y3KfucUihddgi32uha6i6uU3DVGturhJMkMWMELezi9molwpxoElCvvSCaeetH5Z
qFmtHn5lwxn3mtXRCjRXw04sP9sbfux33NsrU7LDAoGAIczglWvhZdRRW0AHcF2H
h5G4Jpv4TTMgUr7N7+/Ws5uI30OCR8+FwSdfuvOkfz7ayvh6XQmyhZnvBWP5mHeW
ETCzvyFmgiPOCs+6v17UNyYMIxDDj33w9F6uNGSjDkoBCA625xnwEIqBPfhvLDfZ
rIGCRl49ih7emIzUrTWAh08=
-----END PRIVATE KEY-----`)

type (
	// testPlugin invokes serveDNSFunc if set. Writes [dns.RcodeServerFailure] otherwise.
	testPlugin struct {
		serveDNSFunc func(w dns.ResponseWriter, r *dns.Msg)
	}

	// testListener is implementation of [net.Listener] that serves [net.Pipe] connections.
	testListener struct {
		mu     sync.Mutex
		cond   sync.Cond
		closed bool
		conns  []net.Conn
	}

	// testConn is utility wrapper over [net.Conn] to exchange DNS and DSO messages.
	testConn struct {
		net.Conn
	}

	// testSession is utility wrapper over [Server].
	//
	// Use with [testing/synctest].
	testServer struct {
		*Server
		plugin *testPlugin
	}
)

func (p *testPlugin) ServeDNS(_ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if p.serveDNSFunc != nil {
		p.serveDNSFunc(w, r)
	} else {
		w.WriteMsg(new(dns.Msg).SetRcode(r, dns.RcodeServerFailure))
	}
	return dns.RcodeSuccess, nil
}

func (p *testPlugin) Name() string { return "test" }

func newTestListener() (l *testListener) {
	l = &testListener{}
	l.cond = *sync.NewCond(&l.mu)
	return l
}

func (l *testListener) Accept() (conn net.Conn, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for {
		if l.closed {
			return nil, errors.New("closed")
		}
		if len(l.conns) > 0 {
			conn = l.conns[0]
			l.conns = l.conns[1:]
			return conn, nil
		}
		l.cond.Wait()
	}
}

func (l *testListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	l.cond.Signal()
	return nil
}

type testAddr struct{}

func (testAddr) Network() string { return "test" }
func (testAddr) String() string  { return "test" }

func (l *testListener) Addr() net.Addr {
	return testAddr{}
}

// addConn adds [net.Pipe] connection that can be accepted.
func (l *testListener) addConn(useTLS bool) *testConn {
	serverConn, clientConn := net.Pipe()
	if useTLS {
		clientConn = ctls.Client(clientConn, &ctls.Config{InsecureSkipVerify: true})
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.conns = append(l.conns, serverConn)
	l.cond.Signal()
	return &testConn{clientConn}
}

func (c *testConn) write(msg []byte) (err error) {
	msg = append(msg, 0, 0)
	copy(msg[2:], msg)
	binary.BigEndian.PutUint16(msg, uint16(len(msg)-2))
	_, err = c.Write(msg)
	return err
}

func (c *testConn) assertWrite(tb testing.TB, msg []byte) {
	tb.Helper()

	err := c.write(msg)
	if err != nil {
		tb.Fatalf("Got %v, want to write %#v", err, msg)
	}
}

func (c *testConn) writeMsg(tb testing.TB, m any) (err error) {
	tb.Helper()

	return c.write(assertPackMsg(tb, m))
}

func (c *testConn) assertWriteMsg(tb testing.TB, m any) {
	tb.Helper()

	err := c.writeMsg(tb, m)
	if err != nil {
		tb.Fatalf("Got %v, want to write %v", err, m)
	}
}

func (c *testConn) read() (msg []byte, err error) {
	var length uint16
	if err := binary.Read(c, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	msg = make([]byte, length)
	if _, err := io.ReadFull(c, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (c *testConn) assertRead(tb testing.TB) (msg []byte) {
	tb.Helper()

	msg, err := c.read()
	if err != nil {
		tb.Fatalf("Got %v, want to read message", err)
	}
	return msg
}

func (c *testConn) readMsg(tb testing.TB) (m any, err error) {
	tb.Helper()

	msg, err := c.read()
	if err != nil {
		return nil, err
	}
	switch rawMsg(msg).opcode() {
	case dns.OpcodeStateful:
		m, err := dsomessage.UnpackMsg(msg, dsomessage.OriginServer)
		if err != nil {
			tb.Fatalf("Got %v, want to unpack DSO message %#v", err, msg)
		}
		return m, nil
	default:
		m := new(dns.Msg)
		err := m.Unpack(msg)
		if err != nil {
			tb.Fatalf("Got %v, want to unpack DNS message %#v", err, msg)
		}
		return m, nil
	}
}

func (c *testConn) assertReadMsg(tb testing.TB) any {
	tb.Helper()

	m, err := c.readMsg(tb)
	if err != nil {
		tb.Fatalf("Got %v, want to read message", err)
	}
	return m
}

func (c *testConn) assertExchangeKeepAlive(tb testing.TB, id uint16, ka dsomessage.KeepAlive) {
	tb.Helper()

	c.assertWriteMsg(tb, dsomessage.NewReqMsg(id, &ka))
	for {
		if m, ok := c.assertReadMsg(tb).(*dsomessage.Msg); ok {
			if m.ID == id {
				if m.Rcode != dns.RcodeSuccess || len(m.TLV) == 0 || m.TLV[0].Type() != dsomessage.TypeKeepAlive {
					tb.Fatalf("Got %v, want KeepAlive response", m)
				}
				return
			}
		}
	}
}

func (c *testConn) assertExchangeSubscribe(tb testing.TB, id uint16, sub dsomessage.Subscribe) {
	tb.Helper()

	c.assertWriteMsg(tb, dsomessage.NewReqMsg(id, &sub))
	for {
		if m, ok := c.assertReadMsg(tb).(*dsomessage.Msg); ok {
			if m.ID == id {
				if m.Rcode != dns.RcodeSuccess {
					tb.Fatalf("Got %#v, want Subscribe response", m)
				}
				return
			}
		}
	}
}

func (c *testConn) assertReadPush(tb testing.TB, change []dns.RR) {
	tb.Helper()

	for {
		if m, ok := c.assertReadMsg(tb).(*dsomessage.Msg); ok {
			if m.ID == 0 && len(m.TLV) > 0 && m.TLV[0].Type() == dsomessage.TypePush {
				got := m.TLV[0].(*dsomessage.Push).Change
				diff := cmp.Diff(change, got, cmp.Comparer(func(a, b dns.RR) bool {
					return a.Header().Ttl == b.Header().Ttl && a.Header().Name == b.Header().Name && dns.IsDuplicate(a, b)
				}))
				if len(diff) > 0 {
					tb.Fatal(diff)
				}
				return
			}
		}
	}
}

func (c *testConn) assertClosed(tb testing.TB, allowInput bool) {
	tb.Helper()

	n, err := io.Copy(io.Discard, c)
	if !allowInput && n > 0 {
		tb.Fatalf("Got n=%v, want 0", n)
	}
	if err != nil {
		tb.Fatalf("Got %v, want closed connection", err)
	}
}

func (c *testConn) assertClosedAfter(tb testing.TB, allowRead bool, d time.Duration) {
	tb.Helper()

	start := time.Now()
	c.assertClosed(tb, allowRead)
	if d1 := time.Since(start); d1 != d {
		tb.Fatalf("Got closed after %v, want %v", d1, d)
	}
}

func (c *testConn) assertAborted(tb testing.TB) {
	tb.Helper()

	c.assertClosedAfter(tb, false, 0)
}

func (s *testServer) start(tb testing.TB, useTLS bool) (*testListener, <-chan error) {
	tb.Helper()

	listener := newTestListener()
	doneC := make(chan error, 1)
	go func() {
		defer close(doneC)
		if useTLS {
			cert, err := ctls.X509KeyPair(localhostCert, localhostKey)
			if err != nil {
				tb.Fatalf("Got %v, want TLS key pair", err)
			}
			s.Config.TLSConfig = &ctls.Config{
				Certificates: []ctls.Certificate{cert},
			}
			doneC <- s.ServeTLS(listener)
		} else {
			doneC <- s.Serve(listener)
		}
	}()
	tb.Cleanup(func() {
		s.Shutdown(tb.Context(), 0)
		<-doneC
	})
	return listener, doneC
}

func (s *testServer) assertStart(tb testing.TB, useTLS bool) *testListener {
	tb.Helper()

	listener, doneC := s.start(tb, useTLS)
	tb.Cleanup(func() {
		s.Shutdown(tb.Context(), 0)
		if err := <-doneC; err != ErrServerClosed {
			tb.Errorf("Got Serve()=%v, want %v", err, ErrServerClosed)
		}
		if len(s.listeners) > 0 {
			tb.Errorf("Got %v, want no listeners", s.listeners)
		}
		if len(s.conns) > 0 {
			tb.Errorf("Got %v, want no connections", s.conns)
		}
	})
	return listener
}

func assertUnpackMsg(tb testing.TB, msg []byte) (m any) {
	tb.Helper()

	var err error
	if rawMsg(msg).opcode() == dns.OpcodeStateful {
		m, err = dsomessage.UnpackMsg(msg, dsomessage.OriginServer)
	} else {
		m = new(dns.Msg)
		err = m.(*dns.Msg).Unpack(msg)
	}
	if err != nil {
		tb.Fatalf("Got %v, want to unpack DSO message", err)
	}
	return m
}

func assertPackMsg(tb testing.TB, m any) (msg []byte) {
	tb.Helper()

	var err error
	switch m := m.(type) {
	case *dsomessage.Msg:
		msg, err = m.Pack()
	case *dns.Msg:
		msg, err = m.Pack()
	default:
		tb.Fatalf("Got m=%v, want DNS or DSO message", m)
	}
	if err != nil {
		tb.Fatalf("Got %v, want to pack message", err)
	}
	return msg
}

func setupServer(tb testing.TB, usePush bool) *testServer {
	tb.Helper()

	upstreamCfg := &dnsserver.Config{
		Zone:        ".",
		Transport:   "dns",
		ListenHosts: []string{"127.0.0.1"},
		Port:        "0",
		Debug:       false,
		Stacktrace:  false,
	}
	upstreamPlugin := &testPlugin{}
	upstreamCfg.AddPlugin(func(plugin.Handler) plugin.Handler { return upstreamPlugin })
	upstream, err := dnsserver.NewServer("127.0.0.1:0", []*dnsserver.Config{upstreamCfg})
	if err != nil {
		tb.Fatalf("Got %v, want CoreDNS server", err)
	}

	cfg := &Config{
		InactivityTimeout:         DefaultInactivityTimeout,
		KeepAliveInterval:         DefaultKeepAliveInterval,
		RestartReconnectInterval:  DefaultRestartReconnectInterval,
		ShutdownReconnectInterval: DefaultShutdownReconnectInterval,
	}
	if usePush {
		cfg.Push = &PushConfig{
			Zones:           []string{"."},
			Classes:         []uint16{dns.ClassINET},
			Types:           []uint16{dns.TypeA},
			RefreshInterval: 0,
			DebounceDelay:   0,
		}
	}
	return &testServer{Server: newServer(cfg, upstream), plugin: upstreamPlugin}
}

func setupServerConn(tb testing.TB, usePush, useTLS bool) (*testServer, *testListener, *testConn) {
	tb.Helper()

	s := setupServer(tb, usePush)
	l := s.assertStart(tb, useTLS)
	c := l.addConn(useTLS)
	synctest.Wait()
	return s, l, c
}

func TestServerShutdown(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		server, listener, conn := setupServerConn(t, false, false)

		server.Shutdown(t.Context(), 0)
		synctest.Wait()

		if !listener.closed {
			t.Error("Want closed listener")
		}

		conn.assertClosed(t, false)

		_, doneC := server.start(t, false)
		if err := <-doneC; err != ErrServerClosed {
			t.Errorf("Got Serve()=%v, want %v", err, ErrServerClosed)
		}

		err := server.Shutdown(t.Context(), 0)
		if err != nil {
			t.Errorf("Got Shutdown()=%v", err)
		}
	})
}

func TestServerShutdownDSO(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		server, _, conn := setupServerConn(t, false, false)

		conn.assertExchangeKeepAlive(t, 1, dsomessage.KeepAlive{})
		synctest.Wait()

		go server.Shutdown(t.Context(), time.Second)
		m := conn.assertReadMsg(t).(*dsomessage.Msg)
		if !m.IsUnidirectional() || len(m.TLV) == 0 || m.TLV[0].Type() != dsomessage.TypeRetryDelay {
			t.Fatalf("Got %v, want RetryDelay TLV unidirectional", m)
		}
		if tlv := m.TLV[0].(*dsomessage.RetryDelay); tlv.RetryDelay != uint32(time.Second.Milliseconds()) {
			t.Errorf("Got RetryDelay=%v, want 42", tlv.RetryDelay)
		}

		conn.assertClosedAfter(t, false, dsosession.SessionGracefulCloseTimeout)
	})
}

func TestServerReadTimeout(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name                 string
		session              bool
		idleTimeout          time.Duration
		readTimeout          time.Duration
		inactivityTimeout    time.Duration
		keepaliveTimeout     time.Duration
		keepaliveMsgInterval time.Duration
		queryMsgInterval     time.Duration
		wantCloseDelay       time.Duration
	}{
		{
			"idle",
			false,
			time.Minute,
			time.Second,
			time.Second,
			time.Second,
			0,
			0,
			time.Minute,
		},
		{
			"read",
			false,
			time.Second,
			time.Minute,
			time.Second,
			time.Second,
			0,
			time.Hour,
			time.Minute,
		},
		{
			"keepalive",
			true,
			time.Second,
			time.Second,
			time.Hour,
			time.Minute,
			0,
			0,
			2 * time.Minute,
		},
		{
			"inactivity",
			true,
			time.Second,
			time.Second,
			time.Minute,
			time.Second,
			time.Second,
			0,
			2 * time.Minute,
		},
		{
			"keepalive never",
			true,
			time.Second,
			time.Second,
			time.Minute,
			dsomessage.KeepAliveIntervalNever * time.Millisecond,
			0,
			0,
			2 * time.Minute,
		},
		{
			"inactivity never",
			true,
			time.Second,
			time.Second,
			dsomessage.InactivityTimeoutNever * time.Millisecond,
			time.Minute,
			0,
			0,
			2 * time.Minute,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				server := setupServer(t, false)
				server.Upstream.IdleTimeout = tc.idleTimeout
				server.Upstream.ReadTimeout = tc.readTimeout
				server.Config.InactivityTimeout = tc.inactivityTimeout
				server.Config.KeepAliveInterval = tc.keepaliveTimeout
				listener := server.assertStart(t, false)
				conn := listener.addConn(false)

				if tc.session {
					conn.assertExchangeKeepAlive(t, 1, dsomessage.KeepAlive{
						InactivityTimeout: uint32(tc.inactivityTimeout.Milliseconds()),
						KeepAliveInterval: uint32(tc.keepaliveTimeout.Milliseconds()),
					})
				}

				if tc.keepaliveMsgInterval > 0 {
					go func() {
						var err error
						for err == nil {
							err = conn.writeMsg(t, dsomessage.NewReqMsg(1, &dsomessage.KeepAlive{
								InactivityTimeout: uint32(tc.inactivityTimeout.Milliseconds()),
								KeepAliveInterval: uint32(tc.keepaliveTimeout.Milliseconds()),
							}))
							select {
							case <-time.After(tc.keepaliveMsgInterval):
							case <-t.Context().Done():
								return
							}
						}
					}()
				}

				if tc.queryMsgInterval > 0 {
					go func() {
						var err error
						for err == nil {
							err = conn.writeMsg(t, new(dns.Msg).SetQuestion("test.", dns.TypeA))
							select {
							case <-time.After(tc.queryMsgInterval):
							case <-t.Context().Done():
								return
							}
						}
					}()
				}

				conn.assertClosedAfter(t, true, tc.wantCloseDelay)
			})
		})
	}

	t.Run("never", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			server, _, conn := setupServerConn(t, true, true)
			server.Config.InactivityTimeout = dsomessage.InactivityTimeoutNever * time.Millisecond
			server.Config.KeepAliveInterval = dsomessage.KeepAliveIntervalNever * time.Millisecond

			conn.assertExchangeSubscribe(t, 1, dsomessage.Subscribe{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET})

			time.Sleep(time.Minute)
			conn.SetReadDeadline(time.Now())
			_, err := conn.readMsg(t)
			if !errors.Is(err, os.ErrDeadlineExceeded) {
				t.Fatalf("Got %v, want functional connection", err)
			}
		})
	})
}

func TestServerTickAlive(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name string
		m    any
	}{
		{
			"dns",
			new(dns.Msg).SetQuestion("test.", dns.TypeA),
		},
		{
			"dso",
			dsomessage.NewReqMsg(1, &dsomessage.Subscribe{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET}),
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				server, _, conn := setupServerConn(t, false, false)
				server.Config.InactivityTimeout = time.Second
				server.Config.KeepAliveInterval = time.Second

				conn.assertExchangeKeepAlive(t, 1, dsomessage.KeepAlive{
					InactivityTimeout: uint32(time.Second.Milliseconds()),
					KeepAliveInterval: uint32(time.Second.Milliseconds()),
				})

				for range 60 {
					conn.assertWriteMsg(t, tc.m)
					conn.assertReadMsg(t)
					time.Sleep(time.Second)
				}
			})
		})
	}
}

func TestServerHandleDNS(t *testing.T) {
	t.Parallel()

	rr, _ := dns.NewRR("test. IN A 192.0.2.1")

	tcs := []struct {
		name      string
		useTLS    bool
		usePush   bool
		wantRcode int
	}{
		{
			"tcp",
			false,
			false,
			dns.RcodeRefused,
		},
		{
			"tcp push",
			false,
			true,
			dns.RcodeRefused,
		},
		{
			"tls",
			true,
			false,
			dns.RcodeRefused,
		},
		{
			"tls push",
			true,
			true,
			dns.RcodeSuccess,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				server, _, conn := setupServerConn(t, tc.usePush, tc.useTLS)
				server.plugin.serveDNSFunc = func(w dns.ResponseWriter, r *dns.Msg) {
					answer := new(dns.Msg).SetReply(r)
					answer.Answer = append(answer.Answer, rr)
					w.WriteMsg(answer)
				}

				conn.assertWriteMsg(t, new(dns.Msg).SetQuestion("test.", dns.TypeA))
				answer := conn.assertReadMsg(t).(*dns.Msg)
				if answer.Rcode != tc.wantRcode {
					t.Fatalf("Got Rcode=%v, want %v", answer.Rcode, tc.wantRcode)
				}
				if answer.Rcode == dns.RcodeSuccess {
					if !dns.IsDuplicate(answer.Answer[0], rr) {
						t.Errorf("Got Answer=%v, want %v", answer.Answer[0], rr)
					}
				}
			})
		})
	}
}

func TestServerHandleBadDNS(t *testing.T) {
	rr, _ := dns.NewRR("test. IN A 192.0.2.1")

	tcs := []struct {
		name       string
		query      *dns.Msg
		acceptFunc dns.MsgAcceptFunc
		wantClose  bool
		wantRcode  int
	}{
		{
			"opcode",
			&dns.Msg{
				MsgHdr: dns.MsgHdr{
					Opcode: dns.OpcodeIQuery,
				},
			},
			dns.DefaultMsgAcceptFunc,
			false,
			dns.RcodeNotImplemented,
		},
		{
			"response",
			&dns.Msg{
				MsgHdr: dns.MsgHdr{
					Response: true,
				},
			},
			dns.DefaultMsgAcceptFunc,
			true,
			0,
		},
		{
			"question",
			&dns.Msg{},
			dns.DefaultMsgAcceptFunc,
			false,
			dns.RcodeFormatError,
		},
		{
			"answer",
			&dns.Msg{
				Question: []dns.Question{{Name: "a.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Answer:   []dns.RR{rr},
			},
			dns.DefaultMsgAcceptFunc,
			false,
			dns.RcodeFormatError,
		},
		{
			"nameserver",
			&dns.Msg{
				Question: []dns.Question{{Name: "a.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Ns:       []dns.RR{rr},
			},
			dns.DefaultMsgAcceptFunc,
			false,
			dns.RcodeFormatError,
		},
		{
			"extra",
			&dns.Msg{
				Question: []dns.Question{{Name: "a.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
				Extra:    []dns.RR{rr, rr, rr},
			},
			dns.DefaultMsgAcceptFunc,
			false,
			dns.RcodeFormatError,
		},
		{
			"DefaultMsgAcceptFunc_MsgRejectNotImplemented",
			&dns.Msg{
				Question: []dns.Question{{Name: "a.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
			},
			func(dns.Header) dns.MsgAcceptAction {
				return dns.MsgRejectNotImplemented
			},
			false,
			dns.RcodeNotImplemented,
		},
		{
			"DefaultMsgAcceptFunc_MsgReject",
			&dns.Msg{
				Question: []dns.Question{{Name: "a.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
			},
			func(dns.Header) dns.MsgAcceptAction {
				return dns.MsgReject
			},
			false,
			dns.RcodeFormatError,
		},
		{
			"DefaultMsgAcceptFunc_Unexpected",
			&dns.Msg{
				Question: []dns.Question{{Name: "a.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
			},
			func(dns.Header) dns.MsgAcceptAction {
				return 42
			},
			true,
			0,
		},
		{
			"zone",
			&dns.Msg{
				Question: []dns.Question{{Name: "b.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}},
			},
			dns.DefaultMsgAcceptFunc,
			false,
			dns.RcodeNotAuth,
		},
		{
			"ixfr",
			&dns.Msg{
				Question: []dns.Question{{Name: "a.test.", Qtype: dns.TypeIXFR, Qclass: dns.ClassINET}},
			},
			dns.DefaultMsgAcceptFunc,
			false,
			dns.RcodeFormatError,
		},
		{
			"axfr",
			&dns.Msg{
				Question: []dns.Question{{Name: "a.test.", Qtype: dns.TypeAXFR, Qclass: dns.ClassINET}},
			},
			dns.DefaultMsgAcceptFunc,
			false,
			dns.RcodeFormatError,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			old := dns.DefaultMsgAcceptFunc
			t.Cleanup(func() {
				dns.DefaultMsgAcceptFunc = old
			})
			dns.DefaultMsgAcceptFunc = tc.acceptFunc

			synctest.Test(t, func(t *testing.T) {
				server, _, conn := setupServerConn(t, true, true)
				server.Config.Push.Zones = []string{"a.test."}

				conn.assertWriteMsg(t, tc.query)
				m, err := conn.readMsg(t)
				if tc.wantClose && err == nil {
					t.Error("Want closed connection")
				} else if !tc.wantClose && m.(*dns.Msg).Rcode != tc.wantRcode {
					t.Errorf("Got Rcode=%v, want %v", m.(*dns.Msg).Rcode, tc.wantRcode)
				}
			})
		})
	}

	t.Run("malformed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			_, _, conn := setupServerConn(t, false, false)

			msg := []byte{0, 13, 0, 0, 0, 0, 0, 1, 0, 1, 0, 1, 0, 1, 42}
			conn.assertWrite(t, msg)
			answer := conn.assertReadMsg(t).(*dns.Msg)
			if answer.Rcode != dns.RcodeFormatError {
				t.Errorf("Got Rcode=%v, want RcodeFormatError", answer.Rcode)
			}
		})
	})
}

func TestServerHandleDNSWithTSIG(t *testing.T) {
	t.Parallel()

	const Secret = "c2VjcmV0Cg=="

	tcs := []struct {
		name        string
		wantBadDNS  bool
		wantBadTsig bool
	}{
		{
			"bad dns - bad tsig",
			true,
			true,
		},
		{
			"bad dns - ok tsig",
			true,
			false,
		},
		{
			"ok dns - bad tsig",
			false,
			true,
		},
		{
			"ok dns - ok tsig",
			false,
			false,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				server, _, conn := setupServerConn(t, true, true)
				server.plugin.serveDNSFunc = func(w dns.ResponseWriter, r *dns.Msg) {
					answer := new(dns.Msg).SetReply(r)
					var rr dns.RR
					if err := w.TsigStatus(); err != nil {
						rr, _ = dns.NewRR("test. IN TXT \"" + err.Error() + "\"")
					} else {
						rr, _ = dns.NewRR("test. IN TXT ok")
					}
					answer.Answer = append(answer.Answer, rr)
					w.WriteMsg(answer)
				}
				if tc.wantBadTsig {
					server.Config.TsigSecret = map[string]string{} // force [dns.RcodeBadKey]
				} else {
					server.Config.TsigSecret = map[string]string{
						"key.test.": Secret,
					}
				}

				query := new(dns.Msg).SetQuestion("test.", dns.TypeTXT)
				if tc.wantBadDNS {
					query.Opcode = dns.OpcodeIQuery // force [dns.RcodeNotImplemented]
				}
				query.Extra = append(query.Extra, &dns.TSIG{
					Hdr:       dns.RR_Header{Name: "key.test.", Rrtype: dns.TypeTSIG, Class: dns.ClassANY},
					Algorithm: dns.HmacSHA1,
					Fudge:     300,
					OrigId:    query.Id,
				})
				msg, queryMAC, err := dns.TsigGenerate(query, Secret, "", false)
				if err != nil {
					t.Fatalf("Got TsigGenerate()=%v", err)
				}

				conn.assertWrite(t, msg)
				msg = conn.assertRead(t)
				answer := assertUnpackMsg(t, msg).(*dns.Msg)

				switch {
				case tc.wantBadDNS && tc.wantBadTsig:
					if answer.Rcode != dns.RcodeNotAuth {
						t.Errorf("Got Rcode=%v, want RcodeNotAuth", answer.Rcode)
					}
					if err := answer.Extra[0].(*dns.TSIG).Error; err != dns.RcodeBadKey {
						t.Errorf("Got TSIG.Error=%v, want RcodeBadKey", err)
					}
				case tc.wantBadDNS:
					if answer.Rcode != dns.RcodeNotImplemented {
						t.Errorf("Got Rcode=%v, want RcodeNotImplemented", answer.Rcode)
					}
					err = dns.TsigVerify(msg, Secret, queryMAC, false)
					if err != nil {
						t.Errorf("Got TsigVerify()=%v", err)
					}
				case tc.wantBadTsig:
					if err := answer.Answer[0].(*dns.TXT).Txt[0]; err != dns.ErrSecret.Error() {
						t.Errorf("Got TsigVerify()=%v, want %v", err, dns.ErrSecret)
					}
				default:
					if err := answer.Answer[0].(*dns.TXT).Txt[0]; err != "ok" {
						t.Errorf("Got TsigVerify()=%v", err)
					}
				}
			})
		})
	}
}

func TestServerHandleDNSWithEDNS0KeepAlive(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name      string
		establish bool
		wantClose bool
	}{
		{
			"dns",
			false,
			false,
		},
		{
			"dso",
			true,
			true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				_, _, conn := setupServerConn(t, true, true)

				if tc.establish {
					conn.assertExchangeKeepAlive(t, 1, dsomessage.KeepAlive{})
				}

				query := new(dns.Msg).SetQuestion("test.", dns.TypeA)
				query.Extra = append(query.Extra, &dns.OPT{
					Hdr: dns.RR_Header{
						Name:   ".",
						Rrtype: dns.TypeOPT,
						Class:  dns.ClassINET,
					},
					Option: []dns.EDNS0{
						&dns.EDNS0_TCP_KEEPALIVE{
							Code:    dns.EDNS0TCPKEEPALIVE,
							Timeout: 1,
						},
					},
				})
				conn.assertWriteMsg(t, query)
				_, err := conn.readMsg(t)
				if tc.wantClose && err == nil {
					t.Error("Want closed connection")
				} else if !tc.wantClose && err != nil {
					t.Errorf("Got %v, want response", err)
				}
			})
		})
	}
}

func TestServerHandleKeepAlive(t *testing.T) {
	t.Parallel()

	t.Run("bad usage", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			_, _, conn := setupServerConn(t, false, false)
			conn.assertExchangeKeepAlive(t, 1, dsomessage.KeepAlive{})
			conn.assertWriteMsg(t, dsomessage.NewUniMsg(&dsomessage.KeepAlive{}))
			conn.assertAborted(t)
		})
	})

	tcs := []struct {
		name         string
		request      dsomessage.KeepAlive
		wantResponse dsomessage.KeepAlive
	}{
		{
			"increase",
			dsomessage.KeepAlive{
				InactivityTimeout: uint32(time.Hour.Milliseconds()),
				KeepAliveInterval: uint32(time.Hour.Milliseconds()),
			},
			dsomessage.KeepAlive{
				InactivityTimeout: uint32(time.Minute.Milliseconds()),
				KeepAliveInterval: uint32(time.Minute.Milliseconds()),
			},
		},
		{
			"decrease",
			dsomessage.KeepAlive{
				InactivityTimeout: uint32(30 * time.Second.Milliseconds()),
				KeepAliveInterval: uint32(30 * time.Second.Milliseconds()),
			},
			dsomessage.KeepAlive{
				InactivityTimeout: uint32(30 * time.Second.Milliseconds()),
				KeepAliveInterval: uint32(30 * time.Second.Milliseconds()),
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				server, _, conn := setupServerConn(t, false, false)
				server.Config.InactivityTimeout = time.Minute
				server.Config.KeepAliveInterval = time.Minute

				conn.assertWriteMsg(t, dsomessage.NewReqMsg(1, &tc.request))
				response := conn.assertReadMsg(t).(*dsomessage.Msg)
				if tlv := response.TLV[0]; !tlv.Equal(&tc.wantResponse) {
					t.Errorf("Got %v, want %v", tlv, tc.wantResponse)
				}
			})
		})
	}
}

func TestServerHandleSubscribe(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name      string
		usePush   bool
		m         *dsomessage.Msg
		wantAbort bool
		wantRcode uint8
	}{
		{
			"push off",
			false,
			dsomessage.NewReqMsg(2, &dsomessage.Subscribe{Name: "a.test.", RRType: dns.TypeA, Class: dns.ClassINET}),
			false,
			dns.RcodeRefused,
		},
		{
			"bad usage",
			true,
			dsomessage.NewUniMsg(&dsomessage.Subscribe{Name: "a.test.", RRType: dns.TypeA, Class: dns.ClassINET}),
			true,
			0,
		},
		{
			"bad zone",
			true,
			dsomessage.NewReqMsg(2, &dsomessage.Subscribe{Name: "b.test.", RRType: dns.TypeA, Class: dns.ClassINET}),
			false,
			dns.RcodeNotAuth,
		},
		{
			"bad rr type",
			true,
			dsomessage.NewReqMsg(2, &dsomessage.Subscribe{Name: "a.test.", RRType: dns.TypeAXFR, Class: dns.ClassINET}),
			false,
			dns.RcodeFormatError,
		},
		{
			"ok",
			true,
			dsomessage.NewReqMsg(2, &dsomessage.Subscribe{Name: "a.test.", RRType: dns.TypeA, Class: dns.ClassINET}),
			false,
			dns.RcodeSuccess,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				server, _, conn := setupServerConn(t, tc.usePush, true)
				if tc.usePush {
					server.Config.Push.Zones = []string{"a.test."}
				}
				conn.assertExchangeKeepAlive(t, 1, dsomessage.KeepAlive{})

				conn.assertWriteMsg(t, tc.m)
				if tc.wantAbort {
					conn.assertAborted(t)
				} else {
					response := conn.assertReadMsg(t).(*dsomessage.Msg)
					if response.Rcode != tc.wantRcode {
						t.Errorf("Got Rcode=%v, want %v", response.Rcode, tc.wantRcode)
					}
					conn.assertClosed(t, false)
				}
			})
		})
	}

	t.Run("duplicate id", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			_, _, conn := setupServerConn(t, true, true)

			tlv1 := dsomessage.Subscribe{Name: "a.test.", RRType: dns.TypeA, Class: dns.ClassINET}
			conn.assertExchangeSubscribe(t, 1, tlv1)

			tlv2 := dsomessage.Subscribe{Name: "a.test.", RRType: dns.TypeA, Class: dns.ClassINET}
			conn.assertWriteMsg(t, dsomessage.NewReqMsg(1, &tlv2))
			conn.assertAborted(t)
		})
	})

	t.Run("duplicate name", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			_, _, conn := setupServerConn(t, true, true)

			tlv1 := dsomessage.Subscribe{Name: "a.test.", RRType: dns.TypeA, Class: dns.ClassINET}
			conn.assertExchangeSubscribe(t, 1, tlv1)

			tlv2 := dsomessage.Subscribe{Name: "a.TeSt.", RRType: dns.TypeA, Class: dns.ClassINET}
			conn.assertWriteMsg(t, dsomessage.NewReqMsg(2, &tlv2))
			conn.assertAborted(t)
		})
	})

	t.Run("duplicate name unicode", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			_, _, conn := setupServerConn(t, true, true)

			var tlv, tlvUnique, tlvDuplicate dsomessage.Subscribe
			err := tlv.UnmarshalBinary([]byte{
				0x12, 0x44, 0x6f, 0x6e, 0x61, 0x6c, 0x64, 0x20, 0x45, 0x2e, 0x20, 0x45, 0x61, 0x73, 0x74, 0x6c, 0x61, 0x6b, 0x65, // Donald E. Eastlake
				0x0c, 0x58, 0x4e, 0x2d, 0x2d, 0x38, 0x30, 0x41, 0x44, 0x58, 0x48, 0x4b, 0x53, // XN--80ADXHKS
				0x05, 0x43, 0x41, 0x46, 0xc3, 0x89, // CAFÉ
				0x04, 0x54, 0x45, 0x53, 0x54, // TEST
				0x00,
				0x00, 0x01, // [dns.TypeA]
				0x00, 0x01, // [dns.ClassINET]
			})
			if err != nil {
				t.Fatalf("Got %v, want to unpack Subscribe", err)
			}
			err = tlvUnique.UnmarshalBinary([]byte{
				0x12, 0x44, 0x6f, 0x6e, 0x61, 0x6c, 0x64, 0x20, 0x45, 0x2e, 0x20, 0x45, 0x61, 0x73, 0x74, 0x6c, 0x61, 0x6b, 0x65, // Donald E. Eastlake
				0x0c, 0x58, 0x4e, 0x2d, 0x2d, 0x38, 0x30, 0x41, 0x44, 0x58, 0x48, 0x4b, 0x53, // XN--80ADXHKS
				0x05, 0x63, 0x61, 0x66, 0xc3, 0xa9, // café
				0x04, 0x54, 0x45, 0x53, 0x54, // TEST
				0x00,
				0x00, 0x01, // [dns.TypeA]
				0x00, 0x01, // [dns.ClassINET]
			})
			if err != nil {
				t.Fatalf("Got %v, want to unpack Subscribe", err)
			}
			err = tlvDuplicate.UnmarshalBinary([]byte{
				0x12, 0x64, 0x6f, 0x6e, 0x61, 0x6c, 0x64, 0x20, 0x65, 0x2e, 0x20, 0x65, 0x61, 0x73, 0x74, 0x6c, 0x61, 0x6b, 0x65, // donald e. eastlake
				0x0c, 0x78, 0x6e, 0x2d, 0x2d, 0x38, 0x30, 0x61, 0x64, 0x78, 0x68, 0x6b, 0x73, // xn--80adxhks
				0x05, 0x43, 0x41, 0x46, 0xc3, 0x89, // CAFÉ
				0x04, 0x74, 0x65, 0x73, 0x74, // test
				0x00,
				0x00, 0x01, // [dns.TypeA]
				0x00, 0x01, // [dns.ClassINET]
			})
			if err != nil {
				t.Fatalf("Got %v, want to unpack Subscribe", err)
			}

			conn.assertExchangeSubscribe(t, 1, tlv)
			conn.assertExchangeSubscribe(t, 2, tlvUnique)
			conn.assertWriteMsg(t, dsomessage.NewReqMsg(3, &tlvDuplicate))
			conn.assertAborted(t)
		})
	})
}

func TestServerHandleUnsubscribe(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name      string
		usePush   bool
		subscribe bool
		m         *dsomessage.Msg
		wantAbort bool
	}{
		{
			"push off",
			false,
			false,
			dsomessage.NewUniMsg(&dsomessage.Unsubscribe{SubscribeID: 2}),
			false,
		},
		{
			"push inactive",
			true,
			false,
			dsomessage.NewUniMsg(&dsomessage.Unsubscribe{SubscribeID: 2}),
			false,
		},
		{
			"bad usage",
			true,
			true,
			dsomessage.NewReqMsg(42, &dsomessage.Unsubscribe{SubscribeID: 2}),
			true,
		},
		{
			"ok",
			true,
			true,
			dsomessage.NewUniMsg(&dsomessage.Unsubscribe{SubscribeID: 2}),
			false,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				_, _, conn := setupServerConn(t, tc.usePush, true)
				conn.assertExchangeKeepAlive(t, 1, dsomessage.KeepAlive{})

				if tc.subscribe {
					conn.assertExchangeSubscribe(t, 2, dsomessage.Subscribe{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET})
				}

				conn.assertWriteMsg(t, tc.m)

				if tc.wantAbort {
					conn.assertAborted(t)
				} else {
					conn.assertClosed(t, false)
				}
			})
		})
	}
}

func TestServerHandleReconfirm(t *testing.T) {
	t.Parallel()

	rr, _ := dns.NewRR("test. IN A 192.0.2.1")

	tcs := []struct {
		name      string
		usePush   bool
		subscribe bool
		m         *dsomessage.Msg
		wantAbort bool
	}{
		{
			"push off",
			false,
			false,
			dsomessage.NewUniMsg(&dsomessage.Reconfirm{RR: rr}),
			false,
		},
		{
			"push inactive",
			true,
			false,
			dsomessage.NewUniMsg(&dsomessage.Reconfirm{RR: rr}),
			false,
		},
		{
			"bad usage",
			true,
			true,
			dsomessage.NewReqMsg(42, &dsomessage.Reconfirm{RR: rr}),
			true,
		},
		{
			"ok",
			true,
			true,
			dsomessage.NewUniMsg(&dsomessage.Reconfirm{RR: rr}),
			false,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				_, _, conn := setupServerConn(t, tc.usePush, true)
				conn.assertExchangeKeepAlive(t, 1, dsomessage.KeepAlive{})

				if tc.subscribe {
					conn.assertExchangeSubscribe(t, 2, dsomessage.Subscribe{Name: "test.", RRType: dns.TypeA, Class: dns.ClassINET})
				}

				conn.assertWriteMsg(t, tc.m)

				if tc.wantAbort {
					conn.assertAborted(t)
				} else {
					conn.assertClosed(t, false)
				}
			})
		})
	}
}

func TestServerHandleBadDSO(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name      string
		msg       []byte
		wantAbort bool
		wantRcode uint8
	}{
		{
			"unexpected response",
			assertPackMsg(t, dsomessage.NewRepMsg(1)),
			true,
			0,
		},
		{
			"malformed tlv header",
			[]byte{0, 1, dns.OpcodeStateful << 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 42},
			true,
			0,
		},
		{
			"unexpected request",
			assertPackMsg(t, dsomessage.NewReqMsg(1, &dsomessage.RetryDelay{})),
			true,
			0,
		},
		{
			"unrecognized request",
			[]byte{0, 1, dns.OpcodeStateful << 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 42, 0, 0},
			false,
			dns.RcodeStatefulTypeNotImplemented,
		},
		{
			"unexpected unidirectional",
			assertPackMsg(t, dsomessage.NewUniMsg(&dsomessage.RetryDelay{})),
			true,
			0,
		},
		{
			"unrecognized unidirectional",
			[]byte{0, 0, dns.OpcodeStateful << 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 42, 0, 0},
			true,
			0,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				_, _, conn := setupServerConn(t, false, false)

				conn.assertExchangeKeepAlive(t, 1, dsomessage.KeepAlive{})

				conn.assertWrite(t, tc.msg)
				if tc.wantAbort {
					conn.assertAborted(t)
				} else {
					m := conn.assertReadMsg(t).(*dsomessage.Msg)
					if m.Rcode != tc.wantRcode {
						t.Errorf("Got Rcode=%v, want %v", m.Rcode, tc.wantRcode)
					}
				}
			})
		})
	}
}

func TestServerHandleDSOWithEncryptionPadding(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name        string
		useTLS      bool
		wantPadding bool
	}{
		{
			"tcp",
			false,
			false,
		},
		{
			"tls",
			true,
			true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				_, _, conn := setupServerConn(t, false, tc.useTLS)

				request := dsomessage.NewReqMsg(1, &dsomessage.KeepAlive{}, &dsomessage.EncryptionPadding{})
				conn.assertWriteMsg(t, request)
				response := conn.assertReadMsg(t).(*dsomessage.Msg)
				i := slices.IndexFunc(response.TLV, func(tlv dsomessage.TLV) bool { return tlv.Type() == dsomessage.TypeEncryptionPadding })
				if tc.wantPadding && i == -1 {
					t.Errorf("Got %v, want EncryptionPadding", response)
				} else if !tc.wantPadding && i != -1 {
					t.Errorf("Got %v, want no EncryptionPadding", response)
				}
			})
		})
	}
}

func TestServerPushLookup(t *testing.T) {
	t.Parallel()

	newRR := func(s string) dns.RR {
		t.Helper()

		rr, err := dns.NewRR(s)
		if err != nil {
			t.Fatalf("Got %v, want %q", err, s)
		}
		return rr
	}

	tcs := []struct {
		name       string
		tlv        dsomessage.Subscribe
		rrs        []dns.RR
		wantChange []dns.RR
	}{
		{
			"cname",
			dsomessage.Subscribe{Name: "A.TEST.", RRType: dns.TypeA, Class: dns.ClassINET},
			[]dns.RR{
				newRR("a.tEsT. IN CNAME b.tEsT."),
			},
			[]dns.RR{
				newRR("A.TEST. IN CNAME b.tEsT."),
			},
		},
		{
			"cname unicode",
			dsomessage.Subscribe{Name: "CAF\\195\\137.TEST.", RRType: dns.TypeA, Class: dns.ClassINET},
			[]dns.RR{
				newRR("cAf\\195\\169.tEsT. IN CNAME a.tEsT."),
				newRR("cAf\\195\\137.TEST. IN CNAME b.TeSt."),
			},
			[]dns.RR{
				newRR("CAF\\195\\137.TEST. IN CNAME b.TeSt."),
			},
		},
		{
			"cname mismatch",
			dsomessage.Subscribe{Name: "A.TEST.", RRType: dns.TypeA, Class: dns.ClassINET},
			[]dns.RR{
				newRR("b.test. IN CNAME c.test."),
				newRR("a.tEsT. IN A 192.0.2.1"),
			},
			[]dns.RR{
				newRR("A.TEST. IN A 192.0.2.1"),
			},
		},
		{
			"cname precedence",
			dsomessage.Subscribe{Name: "A.TEST.", RRType: dns.TypeA, Class: dns.ClassINET},
			[]dns.RR{
				newRR("a.tEsT. IN CNAME b.tEsT."),
				newRR("A.TEST. IN A 192.0.2.1"),
			},
			[]dns.RR{
				newRR("A.TEST. IN CNAME b.tEsT."),
			},
		},
		{
			"dname",
			dsomessage.Subscribe{Name: "A.TEST.", RRType: dns.TypeA, Class: dns.ClassINET},
			[]dns.RR{
				newRR("tEsT. IN DNAME eXaMpLe."),
			},
			[]dns.RR{
				newRR("A.TEST. IN CNAME A.eXaMpLe."),
			},
		},
		{
			"dname unicode",
			dsomessage.Subscribe{Name: "www.CAF\\195\\137.TEST.", RRType: dns.TypeA, Class: dns.ClassINET},
			[]dns.RR{
				newRR("cAf\\195\\169.tEsT. IN DNAME a.eXaMpLe."),
				newRR("cAf\\195\\137.TEST. IN DNAME b.ExAmPlE."),
			},
			[]dns.RR{
				newRR("www.CAF\\195\\137.TEST. IN CNAME www.b.ExAmPlE."),
			},
		},
		{
			"dname mismatch",
			dsomessage.Subscribe{Name: "A.TEST.", RRType: dns.TypeA, Class: dns.ClassINET},
			[]dns.RR{
				newRR("b.test. IN DNAME example."),
				newRR("a.tEsT. IN CNAME c.test."),
			},
			[]dns.RR{
				newRR("A.TEST. IN CNAME c.test."),
			},
		},
		{
			"dname precedence",
			dsomessage.Subscribe{Name: "A.TEST.", RRType: dns.TypeA, Class: dns.ClassINET},
			[]dns.RR{
				newRR("A.TEST. IN CNAME c.test."),
				newRR("tEsT. IN DNAME ExAmPlE."),
			},
			[]dns.RR{
				newRR("A.TEST. IN CNAME A.ExAmPlE."),
			},
		},
		{
			"filter",
			dsomessage.Subscribe{Name: "A.TEST.", RRType: dns.TypeA, Class: dns.ClassINET},
			[]dns.RR{
				newRR("a.test. IN A 192.0.2.1"),
				newRR("b.test. IN A 192.0.2.2"),
				newRR("A.tEsT. IN A 192.0.2.3"),
			},
			[]dns.RR{
				newRR("A.TEST. IN A 192.0.2.1"),
				newRR("A.TEST. IN A 192.0.2.3"),
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				server, _, conn := setupServerConn(t, true, true)
				server.plugin.serveDNSFunc = func(w dns.ResponseWriter, r *dns.Msg) {
					m := new(dns.Msg).SetReply(r)
					m.Answer = tc.rrs
					w.WriteMsg(m)
				}
				conn.assertExchangeSubscribe(t, 1, tc.tlv)
				conn.assertReadPush(t, tc.wantChange)
			})
		})
	}
}
