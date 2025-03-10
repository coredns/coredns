package plugin

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/etcd/msg"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// mockBackend implements ServiceBackend interface for testing
var _ ServiceBackend = &mockBackend{}

type mockBackend struct {
	mockServices func(ctx context.Context, state request.Request, exact bool, opt Options) ([]msg.Service, error)
}

func (m *mockBackend) Serial(state request.Request) uint32 {
	return uint32(time.Now().Unix())
}

func (m *mockBackend) MinTTL(state request.Request) uint32 {
	return 30
}

func (m *mockBackend) Services(ctx context.Context, state request.Request, exact bool, opt Options) ([]msg.Service, error) {
	return m.mockServices(ctx, state, exact, opt)
}

func (m *mockBackend) Reverse(ctx context.Context, state request.Request, exact bool, opt Options) ([]msg.Service, error) {
	return nil, nil
}

func (m *mockBackend) Lookup(ctx context.Context, state request.Request, name string, typ uint16) (*dns.Msg, error) {
	return nil, nil
}

func (m *mockBackend) IsNameError(err error) bool {
	return false
}

func (m *mockBackend) Records(ctx context.Context, state request.Request, exact bool) ([]msg.Service, error) {
	return nil, nil
}

func TestNSStateReset(t *testing.T) {
	// Create a mock backend that always returns error
	mock := &mockBackend{
		mockServices: func(ctx context.Context, state request.Request, exact bool, opt Options) ([]msg.Service, error) {
			return nil, fmt.Errorf("mock error")
		},
	}
	// Create a test request
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeNS)
	state := request.Request{
		Req: req,
		W:   &test.ResponseWriter{},
	}

	originalName := state.QName()
	ctx := context.TODO()

	// Call NS function which should fail due to mock error
	records, extra, err := NS(ctx, mock, "example.org.", state, Options{})

	// Verify error is returned
	if err == nil {
		t.Error("Expected error from mock backend, got nil")
	}

	// Verify query name is reset even when an error occurs
	if state.QName() != originalName {
		t.Errorf("Query name not properly reset after error. Expected %s, got %s", originalName, state.QName())
	}

	// Verify no records are returned
	if len(records) != 0 || len(extra) != 0 {
		t.Error("Expected no records returned on error")
	}
}
