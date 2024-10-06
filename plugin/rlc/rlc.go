package rlc

import (
	"context"
	"encoding/hex"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/golang/groupcache"
	"github.com/jellydator/ttlcache/v3"

	"github.com/miekg/dns"
)

var logger = clog.NewWithPlugin("rlc")

// RlcHandler is the reverse cache handler.
type RlcHandler struct {
	Next plugin.Handler

	TTL       time.Duration
	AnswerTTL time.Duration
	Capacity  int
	UseK8s    bool

	metrics *metrics.Metrics

	UseGroupcache      bool
	CachePort          uint32
	cache              *ttlcache.Cache[string, *PTREntry]
	pool               *groupcache.HTTPPool
	staticPeers        []string
	groupcacheExporter *GroupcacheExporter
	exporter           *Exporter
	cacheServer        *http.Server
	wgCacheServerDone  sync.WaitGroup

	RemoteEnabled      bool
	RemotePort         uint32
	remoteListener     net.Listener
	wgRemoteServerDone sync.WaitGroup

	serviceName string
	serviceNS   string
	self        string
	group       *groupcache.Group
	mx          sync.Mutex
}

const healthPath = "/health"
const cacheName = "rlc"

func (h *RlcHandler) OnReload() error {
	if h.remoteListener != nil {
		h.remoteListener.Close()
		h.wgRemoteServerDone.Wait()
		h.remoteListener = nil
	}
	if h.cacheServer != nil {
		h.cacheServer.Shutdown(context.TODO())
		h.wgCacheServerDone.Wait()
		h.cacheServer = nil
	}
	return h.startup()
}

func (h *RlcHandler) startup() error {
	var err error
	if h.UseGroupcache {
		err = h.initGroupCache()
		if err != nil {
			return err
		}
	}

	if h.RemoteEnabled {
		err = h.initRemote()
		if err != nil {
			return err
		}
	}
	return err
}

func (h *RlcHandler) OnStartup() error {
	h.cache = ttlcache.New[string, *PTREntry](
		ttlcache.WithCapacity[string, *PTREntry](uint64(h.Capacity)),
		ttlcache.WithTTL[string, *PTREntry](h.TTL),
	)
	go h.cache.Start()

	err := h.startup()
	if err != nil {
		return err
	}
	logger.Debug("reverse Lookup Cache started")
	return nil
}

// Name implements the Handler interface.
func (h *RlcHandler) Name() string { return "rlc" }

type ResponseWriter struct {
	dns.ResponseWriter
	handler    *RlcHandler
	remoteAddr net.Addr
}

// RemoteAddr implements the dns.ResponseWriter interface.
func (w *ResponseWriter) RemoteAddr() net.Addr {
	if w.remoteAddr != nil {
		return w.remoteAddr
	}
	return w.ResponseWriter.RemoteAddr()
}

func ptrToIpAddress(ptr string) (net.IP, error) {
	parts := strings.Split(ptr, ".")
	if strings.HasSuffix(ptr, ".in-addr.arpa.") {
		if len(parts) == 7 {
			addr := make([]byte, 4)
			for i := 0; i < 4; i++ {
				b, err := strconv.Atoi(parts[i])
				if err != nil {
					return nil, err
				}
				addr[3-i] = byte(b)
			}
			return net.IPv4(addr[0], addr[1], addr[2], addr[3]), nil
		}
	}
	if strings.HasSuffix(ptr, ".ip6.arpa.") {
		if len(parts) <= 35 && len(parts) > 3 {
			parts = parts[0 : len(parts)-3]
			slices.Reverse(parts)
			addr := make([]byte, 16)
			for i := 0; i < len(parts); i++ {
				hexVal := parts[i]
				b, err := hex.DecodeString(hexVal)
				if err != nil {
					return nil, err
				}
				for x := range b {
					addr[i*2+x] = byte(b[x])
				}
			}
			return net.IP(addr), nil
		}
	}
	return nil, nil
}

func (h *RlcHandler) handleRemotePtrMessage(res *dns.Msg) error {
	h.mx.Lock()
	defer h.mx.Unlock()
	ts := time.Now()

	for _, answer := range res.Answer {
		var ptr *dns.PTR
		var ok bool
		if ptr, ok = answer.(*dns.PTR); !ok {
			continue
		}

		ip, err := ptrToIpAddress(answer.Header().Name)
		if err != nil {
			return err
		}
		var entry *PTREntry
		item := h.cache.Get(ip.String())
		if item == nil {
			entry = newPTREntry(ip, ts)
			h.cache.Set(ip.String(), entry, h.TTL)
		} else {
			entry = item.Value()
		}
		entry.TouchName(ptr.Ptr, ts)
	}
	return nil
}

func (h *RlcHandler) handleQueryMessage(res *dns.Msg) {
	names := make([]string, 1)
	names[0] = res.Question[0].Name
	for _, rr := range res.Answer {
		if cname, ok := rr.(*dns.CNAME); ok {
			names = append(names, cname.Target)
		}
	}
	for _, rr := range res.Answer {
		if a, ok := rr.(*dns.A); ok {
			h.mx.Lock()
			ts := time.Now()

			var entry *PTREntry
			item := h.cache.Get(a.A.String())
			if item == nil {
				entry = newPTREntry(a.A, ts)
				h.cache.Set(a.A.String(), entry, h.TTL)
			} else {
				entry = item.Value()
			}
			entry.AddName(a.Hdr.Name, ts)
			for i, name := range names {
				if name != a.Hdr.Name {
					if i == 0 {
						entry.TouchName(a.Hdr.Name, ts)
					} else {
						entry.AddName(name, ts)
					}
				}
			}
			h.mx.Unlock()
			continue
		}

		if a, ok := rr.(*dns.AAAA); ok {
			h.mx.Lock()
			ts := time.Now()

			var entry *PTREntry
			item := h.cache.Get(a.AAAA.String())
			if item == nil {
				entry = newPTREntry(a.AAAA, ts)
				h.cache.Set(a.AAAA.String(), entry, h.TTL)
			} else {
				entry = item.Value()
			}
			entry.AddName(a.Hdr.Name, ts)
			for i, name := range names {
				if name != a.Hdr.Name {
					if i == 0 {
						entry.TouchName(a.Hdr.Name, ts)
					} else {
						entry.AddName(name, ts)
					}
				}
			}
			h.mx.Unlock()
			continue
		}
	}
}

// WriteMsg implements the dns.ResponseWriter interface.
func (w *ResponseWriter) WriteMsg(res *dns.Msg) error {
	if len(res.Question) == 1 {
		if res.Question[0].Qtype == dns.TypePTR {
			ip, err := ptrToIpAddress(res.Question[0].Name)
			if err != nil {
				return err
			}
			item := w.handler.cache.Get(ip.String())
			if item != nil {
				newRes := res.Copy()
				names := NameMap(item.Value().Names).getNames()
				answers := make([]dns.RR, len(names)+len(res.Answer))
				for i, name := range names {
					answers[i] = &dns.PTR{
						Hdr: dns.RR_Header{
							Name:   res.Question[0].Name,
							Class:  dns.ClassINET,
							Rrtype: dns.TypePTR,
							Ttl:    uint32(w.handler.AnswerTTL.Seconds()),
						},
						Ptr: name.Name,
					}

				}
				for i, origAnswer := range res.Answer {
					answers[len(names)+i] = origAnswer
				}
				newRes.Answer = answers
				return w.ResponseWriter.WriteMsg(newRes)
			}
			return w.ResponseWriter.WriteMsg(res)
		}
		if res.MsgHdr.Response && (res.Question[0].Qtype == dns.TypeA || res.Question[0].Qtype == dns.TypeAAAA) {
			w.handler.handleQueryMessage(res)
		}
	}
	return w.ResponseWriter.WriteMsg(res)
}

// Write implements the dns.ResponseWriter interface.
func (w *ResponseWriter) Write(buf []byte) (int, error) {
	logger.Warning("Rlc called with Write: not caching reply")
	n, err := w.ResponseWriter.Write(buf)
	return n, err
}

func (h *RlcHandler) isMessageOfInterest(r *dns.Msg) bool {
	if len(r.Question) != 1 || (r.Question[0].Qtype != dns.TypeA && r.Question[0].Qtype != dns.TypeAAAA && r.Question[0].Qtype != dns.TypePTR) {
		return false
	}
	return true
}

func (h *RlcHandler) isQueryMessageOfInterest(r *dns.Msg) bool {
	return r.Response && len(r.Answer) > 0 && len(r.Question) == 1 && (r.Question[0].Qtype == dns.TypeA || r.Question[0].Qtype == dns.TypeAAAA)
}

func (h *RlcHandler) isPtrMessageOfInterest(r *dns.Msg) bool {
	if len(r.Question) != 1 || (r.Question[0].Qtype != dns.TypePTR) {
		return false
	}
	return true
}

// ServeDNS implements the plugin.Handle interface.
func (h *RlcHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if !h.isMessageOfInterest(r) {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	rc := r.Copy() // We potentially modify r, to prevent other plugins from seeing this (r is a pointer), copy r into rc.
	catchWriter := &ResponseWriter{
		ResponseWriter: w,
		handler:        h,
		remoteAddr:     nil,
	}
	return plugin.NextOrFailure(h.Name(), h.Next, ctx, catchWriter, rc)
}

func newRlcHander() *RlcHandler {
	return &RlcHandler{
		TTL:           3600,
		AnswerTTL:     5,
		Capacity:      2048,
		UseGroupcache: false,
		serviceName:   "coredns-groupcache",
		CachePort:     8000,
		RemoteEnabled: false,
		RemotePort:    8053,
	}
}
