package hanzodns

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/reuseport"
)

var log = clog.NewWithPlugin("hanzodns")

func init() { plugin.Register("hanzodns", setup) }

func setup(c *caddy.Controller) error {
	addr, err := parse(c)
	if err != nil {
		return plugin.Error("hanzodns", err)
	}

	store := NewStore()

	h := &HanzoDNS{
		Store: store,
		Addr:  addr,
	}

	srv := &apiServer{
		addr:  addr,
		store: store,
	}

	c.OnStartup(srv.OnStartup)
	c.OnRestart(srv.OnShutdown)
	c.OnFinalShutdown(srv.OnShutdown)

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		h.Next = next
		return h
	})

	return nil
}

func parse(c *caddy.Controller) (string, error) {
	addr := ":8443"
	for c.Next() {
		args := c.RemainingArgs()
		switch len(args) {
		case 0:
		case 1:
			addr = args[0]
			if _, _, err := net.SplitHostPort(addr); err != nil {
				return "", err
			}
		default:
			return "", c.ArgErr()
		}
		// No block directives for now.
		for c.NextBlock() {
			return "", c.ArgErr()
		}
	}
	return addr, nil
}

// apiServer manages the lifecycle of the HTTP API server.
type apiServer struct {
	addr  string
	store *Store
	ln    net.Listener
	srv   *http.Server
	setup bool
}

const shutdownTimeout = 5 * time.Second

func (s *apiServer) OnStartup() error {
	ln, err := reuseport.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln
	s.setup = true

	mux := http.NewServeMux()
	registerRoutes(mux, s.store)

	s.srv = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Infof("Starting Hanzo DNS API on %s", s.addr)
	go func() { s.srv.Serve(s.ln) }()

	return nil
}

func (s *apiServer) OnShutdown() error {
	if !s.setup {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := s.srv.Shutdown(ctx); err != nil {
		log.Errorf("Failed to stop Hanzo DNS API server: %s", err)
	}
	s.setup = false
	return nil
}
