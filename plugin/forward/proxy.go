/*
 * Copyright (c) 2016 Felipe Cavalcanti <fjfcavalcanti@gmail.com>
 * Author: Felipe Cavalcanti <fjfcavalcanti@gmail.com>
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of
 * this software and associated documentation files (the "Software"), to deal in
 * the Software without restriction, including without limitation the rights to
 * use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
 * the Software, and to permit persons to whom the Software is furnished to do so,
 * subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
 * FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
 * COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
 * IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
 * CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

/* General idea kept, but completely rewritten for use in CoreDNS */

package forward

import (
	"net"
	"sync"
	"time"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type conn struct {
	c    *dns.Conn
	w    dns.ResponseWriter
	used time.Time
}

type proxy struct {
	host        *host
	connTimeout time.Duration
	conns       map[string]conn
	sync.RWMutex
	clientChan   chan request.Request
	upstreamChan chan request.Request

	// copied from Forward.
	hcInterval time.Duration
	forceTCP   bool

	cl   bool
	clMu sync.RWMutex
}

func newProxy(addr string) *proxy {
	proxy := &proxy{
		host:         newHost(addr),
		connTimeout:  connTimeout,
		cl:           false,
		conns:        make(map[string]conn),
		clientChan:   make(chan request.Request),
		upstreamChan: make(chan request.Request),
		hcInterval:   hcDuration,
	}

	return proxy
}

func (p *proxy) closed() bool {
	p.clMu.RLock()
	b := p.cl
	p.clMu.RUnlock()
	return b
}

func (p *proxy) setClosed(b bool) {
	p.clMu.Lock()
	p.cl = b
	p.clMu.Unlock()
}

func (p *proxy) setUsed(clientID string) {
	p.Lock()
	if _, found := p.conns[clientID]; found {
		connWrapper := p.conns[clientID]
		connWrapper.used = time.Now()
		p.conns[clientID] = connWrapper
	}
	p.Unlock()
}

// clientRead reads from upstream and sends it to upstreamChan for writing it
// back to the original client.
func (p *proxy) clientRead(upstreamConn *dns.Conn, w dns.ResponseWriter) {
	clientID, _ := clientID(w)
	for {
		start := time.Now()
		ret, err := upstreamConn.ReadMsg()
		if err != nil {
			p.Lock()
			upstreamConn.Close()
			delete(p.conns, clientID)
			p.Unlock()
			return
		}

		p.setUsed(clientID)
		state := request.Request{Req: ret, W: w}
		p.upstreamChan <- state

		RequestDuration.WithLabelValues(state.Proto(), familyToString(state.Family()), p.host.String()).Observe(float64(time.Since(start) / time.Millisecond))
	}
}

func (p *proxy) upstreamPackets() {
	for pa := range p.upstreamChan {
		pa.W.WriteMsg(pa.Req)
	}
}

func (p *proxy) clientPackets() {
	for pa := range p.clientChan {
		clientID, proto := clientID(pa.W)

		p.RLock()
		c, found := p.conns[clientID]
		p.RUnlock()

		if !found {
			if p.forceTCP {
				proto = "tcp"
			}
			c, err := dns.DialTimeout(proto, p.host.addr, dialTimeout)
			if err != nil {
				// try another nameserver?
				continue
			}

			p.Lock()
			p.conns[clientID] = conn{
				c:    c,
				w:    pa.W,
				used: time.Now(),
			}
			p.Unlock()

			c.WriteMsg(pa.Req)

			go p.clientRead(c, pa.W)
			continue
		}

		c.c.WriteMsg(pa.Req)

		p.RLock()
		if _, ok := p.conns[clientID]; ok {
			if p.conns[clientID].used.Before(
				time.Now().Add(-p.connTimeout / 4)) {
				p.setUsed(clientID)
			}
		}
		p.RUnlock()
	}
}

func (p *proxy) free() {
	for !p.closed() {
		time.Sleep(p.connTimeout)

		p.Lock()
		for client, conn := range p.conns {
			if conn.used.Before(time.Now().Add(-p.connTimeout)) {
				delete(p.conns, client)
			}
		}
		p.Unlock()
	}
}

func (p *proxy) healthCheck() {
	for !p.closed() {
		time.Sleep(p.hcInterval)

		go p.host.Check()
	}
}

// knownClient returns true when this particular client has been seen by this proxy.
func (p *proxy) knownClient(id string) bool {
	p.RLock()
	_, ok := p.conns[id]
	p.RUnlock()
	return ok
}

// Len returns the number of known connection for this proxy.
func (p *proxy) Len() int {
	p.RLock()
	l := len(p.conns)
	p.RUnlock()
	return l
}

// clientID returns a string that identifies this particular client's 3-tuple.
func clientID(w dns.ResponseWriter) (id, proto string) {
	if _, ok := w.RemoteAddr().(*net.UDPAddr); ok {
		return w.RemoteAddr().String() + "udp", "udp"
	}
	return w.RemoteAddr().String() + "tcp", "tcp"
}

const (
	dialTimeout = 1 * time.Second
	connTimeout = 3500 * time.Millisecond
	hcDuration  = 500 * time.Millisecond
)
