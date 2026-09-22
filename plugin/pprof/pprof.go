// Package pprof implements a debug endpoint for getting profiles using the
// go pprof tooling.
package pprof

import (
	"context"
	"net"
	"net/http"
	pp "net/http/pprof"
	"runtime"
	"time"

	"github.com/coredns/coredns/plugin/pkg/reuseport"
)

type handler struct {
	addr     string
	rateBloc int
	ln       net.Listener
	srv      *http.Server
	mux      *http.ServeMux
}

const shutdownTimeout = 5 * time.Second

func (h *handler) Startup() error {
	// Reloading the plugin without changing the listening address results
	// in an error unless we reuse the port because Startup is called for
	// new handlers before Shutdown is called for the old ones.
	ln, err := reuseport.Listen("tcp", h.addr)
	if err != nil {
		log.Errorf("Failed to start pprof handler: %s", err)
		return err
	}

	h.ln = ln

	h.mux = http.NewServeMux()
	h.mux.HandleFunc(path, func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, path+"/", http.StatusFound)
	})
	h.mux.HandleFunc(path+"/", pp.Index)
	h.mux.HandleFunc(path+"/cmdline", pp.Cmdline)
	h.mux.HandleFunc(path+"/profile", pp.Profile)
	h.mux.HandleFunc(path+"/symbol", pp.Symbol)
	h.mux.HandleFunc(path+"/trace", pp.Trace)

	runtime.SetBlockProfileRate(h.rateBloc)

	h.srv = &http.Server{
		Handler:      h.mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  5 * time.Second,
	}

	go func() { h.srv.Serve(h.ln) }()
	return nil
}

func (h *handler) Shutdown() error {
	if h.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := h.srv.Shutdown(ctx); err != nil {
			log.Infof("Failed to stop pprof http server: %s", err)
			return err
		}
	}
	return nil
}

const (
	path = "/debug/pprof"
)
