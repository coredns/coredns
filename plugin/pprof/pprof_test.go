package pprof

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHandlerStartup(t *testing.T) {
	h := &handler{
		addr:     ":0", // Use available port
		rateBloc: 5,
	}

	err := h.Startup()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer h.Shutdown()

	if h.ln == nil {
		t.Fatal("Expected listener to be set")
	}

	if h.mux == nil {
		t.Fatal("Expected mux to be set")
	}

	// Verify the server is actually listening
	addr := h.ln.Addr().String()
	if addr == "" {
		t.Fatal("Expected non-empty address")
	}
}

func TestHandlerShutdown(t *testing.T) {
	h := &handler{
		addr:     ":0",
		rateBloc: 1,
	}

	// Start the handler
	err := h.Startup()
	if err != nil {
		t.Fatalf("Expected no error during startup, got: %v", err)
	}

	// Verify listener exists
	if h.ln == nil {
		t.Fatal("Expected listener to be set after startup")
	}

	// Shutdown and verify no error
	err = h.Shutdown()
	if err != nil {
		t.Errorf("Expected no error during shutdown, got: %v", err)
	}
}

func TestHandlerShutdownWithoutStartup(t *testing.T) {
	h := &handler{}

	// Shutdown without startup should not error
	err := h.Shutdown()
	if err != nil {
		t.Errorf("Expected no error when shutting down without startup, got: %v", err)
	}
}

func TestHandlerPprofEndpoints(t *testing.T) {
	h := &handler{
		addr:     ":0",
		rateBloc: 1,
	}

	err := h.Startup()
	if err != nil {
		t.Fatalf("Expected no error during startup, got: %v", err)
	}
	defer h.Shutdown()

	// Wait a bit for the server to fully start
	time.Sleep(100 * time.Millisecond)

	baseURL := fmt.Sprintf("http://%s", h.ln.Addr().String())

	testCases := []struct {
		path           string
		expectedStatus int
	}{
		{"/debug/pprof/", http.StatusOK},        // Index page
		{"/debug/pprof/cmdline", http.StatusOK}, // Cmdline
		{"/debug/pprof/symbol", http.StatusOK},  // Symbol
	}

	for _, tc := range testCases {
		url := baseURL + tc.path
		resp, err := http.Get(url)
		if err != nil {
			t.Errorf("Error making request to %s: %v", url, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != tc.expectedStatus {
			t.Errorf("Expected status %d for %s, got %d", tc.expectedStatus, tc.path, resp.StatusCode)
		}
	}
}

func TestHandlerPprofRedirect(t *testing.T) {
	h := &handler{
		addr:     ":0",
		rateBloc: 1,
	}

	err := h.Startup()
	if err != nil {
		t.Fatalf("Expected no error during startup, got: %v", err)
	}
	defer h.Shutdown()

	// Wait a bit for the server to fully start
	time.Sleep(100 * time.Millisecond)

	// Create a client that doesn't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	url := fmt.Sprintf("http://%s/debug/pprof", h.ln.Addr().String())
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("Expected status %d, got %d", http.StatusFound, resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if !strings.HasSuffix(location, "/debug/pprof/") {
		t.Errorf("Expected redirect to end with '/debug/pprof/', got: %s", location)
	}
}

func TestHandlerStartupInvalidAddress(t *testing.T) {
	h := &handler{
		addr:     "invalid-address-format",
		rateBloc: 1,
	}

	err := h.Startup()
	if err == nil {
		t.Fatal("Expected error for invalid address format")
		defer h.Shutdown()
	}
}
