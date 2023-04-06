package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func Test_health_overloaded_cancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	h := &health{
		Addr: ts.URL,
		stop: cancel,
	}

	var err error
	h.healthURI, err = url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	h.healthURI.Path = "/health"

	stopped := make(chan struct{})
	go func() {
		h.overloaded(ctx)
		stopped <- struct{}{}
	}()

	// wait for overloaded function to start atleast once
	time.Sleep(1 * time.Second)

	cancel()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("overloaded function should have been cancelled")
	}
}
