// Package cache implements a cache.
package cache

import (
	"bytes"
	"encoding/binary"
	"net"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/cache"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Cache is plugin that looks up responses in a cache and caches replies.
// It has a success and a denial of existence cache.
type Cache struct {
	Next  plugin.Handler
	Zones []string

	ncache *cache.Cache
	ncap   int
	nttl   time.Duration

	pcache *cache.Cache
	pcap   int
	pttl   time.Duration

	// Prefetch.
	prefetch   int
	duration   time.Duration
	percentage int

	// Testing.
	now func() time.Time
}

// New returns an initialized Cache with default settings. It's up to the
// caller to set the Next handler.
func New() *Cache {
	return &Cache{
		Zones:      []string{"."},
		pcap:       defaultCap,
		pcache:     cache.New(defaultCap),
		pttl:       maxTTL,
		ncap:       defaultCap,
		ncache:     cache.New(defaultCap),
		nttl:       maxNTTL,
		prefetch:   0,
		duration:   1 * time.Minute,
		percentage: 10,
		now:        time.Now,
	}
}

func uint16ToWire(data uint16) string {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, uint16(data))
	return string(buf)
}

func optToString(rrs []dns.RR) string {
	h := bytes.Buffer{}
	buf := make([]byte, 256)
	for _, r := range rrs {
		if r.Header().Rrtype == dns.TypeOPT {
			off, err := dns.PackRR(r, buf, 0, nil, false)
			if err == nil {
				h.Write(buf[:off])
			}
		}
	}

	return h.String()
}

// Return key under which we store the item, -1 will be returned if we don't store the
// message.
// Currently we do not cache Truncated, errors zone transfers or dynamic update messages.
func key(m *dns.Msg, t response.Type, do bool) string {
	// We don't store truncated responses.
	if m.Truncated {
		return ""
	}
	// Nor errors or Meta or Update
	if t == response.OtherError || t == response.Meta || t == response.Update {
		return ""
	}
	// uint16ToWire writes unit16 to wire/binary format

	id := m.Question[0].Name + "|" + uint16ToWire(m.Question[0].Qtype)
	if do {
		id += "|DO"
	}
	id += optToString(m.Extra)

	return id
}

// ResponseWriter is a response writer that caches the reply message.
type ResponseWriter struct {
	dns.ResponseWriter
	*Cache
	state  request.Request
	server string // Server handling the request.

	prefetch   bool // When true write nothing back to the client.
	remoteAddr net.Addr
}

// newPrefetchResponseWriter returns a Cache ResponseWriter to be used in
// prefetch requests. It ensures RemoteAddr() can be called even after the
// original connetion has already been closed.
func newPrefetchResponseWriter(server string, state request.Request, c *Cache) *ResponseWriter {
	// Resolve the address now, the connection might be already closed when the
	// actual prefetch request is made.
	addr := state.W.RemoteAddr()
	// The protocol of the client triggering a cache prefetch doesn't matter.
	// The address type is used by request.Proto to determine the response size,
	// and using TCP ensures the message isn't unnecessarily truncated.
	if u, ok := addr.(*net.UDPAddr); ok {
		addr = &net.TCPAddr{IP: u.IP, Port: u.Port, Zone: u.Zone}
	}

	return &ResponseWriter{
		ResponseWriter: state.W,
		Cache:          c,
		state:          state,
		server:         server,
		prefetch:       true,
		remoteAddr:     addr,
	}
}

// RemoteAddr implements the dns.ResponseWriter interface.
func (w *ResponseWriter) RemoteAddr() net.Addr {
	if w.remoteAddr != nil {
		return w.remoteAddr
	}
	return w.ResponseWriter.RemoteAddr()
}

// WriteMsg implements the dns.ResponseWriter interface.
func (w *ResponseWriter) WriteMsg(res *dns.Msg) error {
	do := false
	mt, opt := response.Typify(res, w.now().UTC())
	if opt != nil {
		do = opt.Do()
	}

	// key returns empty string for anything we don't want to cache.
	key := key(res, mt, do)

	duration := w.pttl
	if mt == response.NameError || mt == response.NoData {
		duration = w.nttl
	}

	msgTTL := dnsutil.MinimalTTL(res, mt)
	if msgTTL < duration {
		duration = msgTTL
	}

	if key != "" && duration > 0 {
		if w.state.Match(res) {
			w.set(res, key, mt, duration)
			cacheSize.WithLabelValues(w.server, Success).Set(float64(w.pcache.Len()))
			cacheSize.WithLabelValues(w.server, Denial).Set(float64(w.ncache.Len()))
		} else {
			// Don't log it, but increment counter
			cacheDrops.WithLabelValues(w.server).Inc()
		}
	}

	if w.prefetch {
		return nil
	}

	// Apply capped TTL to this reply to avoid jarring TTL experience 1799 -> 8 (e.g.)
	ttl := uint32(duration.Seconds())
	for i := range res.Answer {
		res.Answer[i].Header().Ttl = ttl
	}
	for i := range res.Ns {
		res.Ns[i].Header().Ttl = ttl
	}
	for i := range res.Extra {
		if res.Extra[i].Header().Rrtype != dns.TypeOPT {
			res.Extra[i].Header().Ttl = ttl
		}
	}
	return w.ResponseWriter.WriteMsg(res)
}

func (w *ResponseWriter) set(m *dns.Msg, key string, mt response.Type, duration time.Duration) {
	if key == "" || duration == 0 {
		return
	}

	switch mt {
	case response.NoError, response.Delegation:
		i := newItem(m, w.now(), duration)
		w.pcache.Add(key, i)

	case response.NameError, response.NoData:
		i := newItem(m, w.now(), duration)
		w.ncache.Add(key, i)

	case response.OtherError:
		// don't cache these
	default:
		log.Warningf("Caching called with unknown classification: %d", mt)
	}
}

// Write implements the dns.ResponseWriter interface.
func (w *ResponseWriter) Write(buf []byte) (int, error) {
	log.Warning("Caching called with Write: not caching reply")
	if w.prefetch {
		return 0, nil
	}
	n, err := w.ResponseWriter.Write(buf)
	return n, err
}

const (
	maxTTL  = dnsutil.MaximumDefaulTTL
	maxNTTL = dnsutil.MaximumDefaulTTL / 2

	defaultCap = 10000 // default capacity of the cache.

	// Success is the class for caching positive caching.
	Success = "success"
	// Denial is the class defined for negative caching.
	Denial = "denial"
)
