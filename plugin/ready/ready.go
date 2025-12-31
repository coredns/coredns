// Package ready is used to signal readiness of the CoreDNS process. Once all
// plugins have called in the plugin will signal readiness by returning a 200
// OK on the HTTP handler (on port 8181). If not ready yet, the handler will
// return a 503.
package ready

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/reuseport"
	"github.com/coredns/coredns/plugin/pkg/uniq"
)

var (
	log      = clog.NewWithPlugin("ready")
	plugins  = &list{}
	uniqAddr = uniq.New()
)

type ready struct {
	Addr string

	sync.RWMutex
	ln   net.Listener
	srv  *http.Server
	done bool
	mux  *http.ServeMux
}

const shutdownTimeout = 5 * time.Second

func (rd *ready) onStartup() error {
	ln, err := reuseport.Listen("tcp", rd.Addr)
	if err != nil {
		return err
	}

	rd.Lock()
	rd.ln = ln
	rd.mux = http.NewServeMux()
	rd.done = true
	rd.Unlock()

	rd.mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		rd.Lock()
		defer rd.Unlock()
		if !rd.done {
			w.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(w, "Shutting down")
			return
		}
		ready, notReadyPlugins := plugins.Ready()
		if ready {
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, http.StatusText(http.StatusOK))
			return
		}
		log.Infof("Plugins not ready: %q", notReadyPlugins)
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, notReadyPlugins)
	})

	rd.srv = &http.Server{
		Handler:      rd.mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  5 * time.Second,
	}

	go func() { rd.srv.Serve(rd.ln) }()

	return nil
}

func (rd *ready) onFinalShutdown() error {
	rd.Lock()
	defer rd.Unlock()
	if !rd.done {
		return nil
	}

	uniqAddr.Unset(rd.Addr)

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := rd.srv.Shutdown(ctx); err != nil {
		log.Infof("Failed to stop ready http server: %s", err)
	}
	rd.done = false
	return nil
}
