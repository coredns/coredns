package forwardcrd

import (
	"context"
	"sync"

	"github.com/coredns/coredns/plugin/forward"

	"github.com/miekg/dns"
)

type TestPluginHandler struct {
	mutex               sync.Mutex
	ReceivedConfig      forward.ForwardConfig
	onStartupCallCount  int
	onShutdownCallCount int
}

func (t *TestPluginHandler) ServeDNS(context.Context, dns.ResponseWriter, *dns.Msg) (int, error) {
	return 0, nil
}

func (t *TestPluginHandler) Name() string { return "" }

func (t *TestPluginHandler) OnStartup() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.onStartupCallCount++
	return nil
}

func (t *TestPluginHandler) OnShutdown() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.onShutdownCallCount++
	return nil
}

func (t *TestPluginHandler) OnStartupCallCount() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.onStartupCallCount
}

func (t *TestPluginHandler) OnShutdownCallCount() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.onShutdownCallCount
}

type TestPluginInstancer struct {
	mutex              sync.Mutex
	testPluginHandlers []*TestPluginHandler
}

func (t *TestPluginInstancer) NewWithConfig(config forward.ForwardConfig) (lifecyclePluginHandler, error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	testPluginHandler := &TestPluginHandler{
		ReceivedConfig: config,
	}
	t.testPluginHandlers = append(t.testPluginHandlers, testPluginHandler)
	return testPluginHandler, nil
}

func (t *TestPluginInstancer) NewWithConfigArgsForCall(index int) *TestPluginHandler {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	return t.testPluginHandlers[index]
}

func (t *TestPluginInstancer) NewWithConfigCallCount() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	return len(t.testPluginHandlers)
}

type TestController struct {
}

func (t *TestController) Run(threads int) {}
func (t *TestController) HasSynced() bool { return true }
func (t *TestController) Stop() error     { return nil }
