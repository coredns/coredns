package health

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestHealth(t *testing.T) {
	h := &health{Addr: ":0"}

	if err := h.OnStartup(); err != nil {
		t.Fatalf("Unable to startup the health server: %v", err)
	}
	defer h.OnFinalShutdown()

	address := fmt.Sprintf("http://%s%s", h.ln.Addr().String(), "/health")

	response, err := http.Get(address)
	if err != nil {
		t.Fatalf("Unable to query %s: %v", address, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("Invalid status code: expecting '200', got '%d'", response.StatusCode)
	}
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("Unable to get response body from %s: %v", address, err)
	}
	response.Body.Close()

	if string(content) != http.StatusText(http.StatusOK) {
		t.Errorf("Invalid response body: expecting 'OK', got '%s'", string(content))
	}
}

func TestHealthLameduck(t *testing.T) {
	h := &health{Addr: ":0", lameduck: 250 * time.Millisecond}

	if err := h.OnStartup(); err != nil {
		t.Fatalf("Unable to startup the health server: %v", err)
	}

	h.OnFinalShutdown()
}
