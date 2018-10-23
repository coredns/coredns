// Package mdns implements multicast DNS service.
package mdns

import (
	"net"
	"net/http"
)

type handler struct {
	addr string
	ln   net.Listener
	mux  *http.ServeMux
}

func (h *handler) Startup() error {
	ln, err := net.Listen("tcp", h.addr)
	if err != nil {
		log.Errorf("Failed to start mdns handler: %s", err)
		return err
	}

	h.ln = ln

	h.mux = http.NewServeMux()

	go func() {
		http.Serve(h.ln, h.mux)
	}()
	return nil
}

func (h *handler) Shutdown() error {
	if h.ln != nil {
		return h.ln.Close()
	}
	return nil
}
