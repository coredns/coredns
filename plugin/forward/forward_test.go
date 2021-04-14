package forward

import (
	"crypto/tls"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/dnstap"
)

func TestList(t *testing.T) {
	f := Forward{
		proxies: []*Proxy{{addr: "1.1.1.1:53"}, {addr: "2.2.2.2:53"}, {addr: "3.3.3.3:53"}},
		p:       &roundRobin{},
	}

	expect := []*Proxy{{addr: "2.2.2.2:53"}, {addr: "1.1.1.1:53"}, {addr: "3.3.3.3:53"}}
	got := f.List()

	if len(got) != len(expect) {
		t.Fatalf("Expected: %v results, got: %v", len(expect), len(got))
	}
	for i, p := range got {
		if p.addr != expect[i].addr {
			t.Fatalf("Expected proxy %v to be '%v', got: '%v'", i, expect[i].addr, p.addr)
		}
	}
}

func TestNewWithConfig(t *testing.T) {
	expectedExcept := []string{"foo.com.", "example.com."}
	expectedMaxFails := uint32(5)
	expectedHealthCheck := 5 * time.Second
	expectedServerName := "test"
	expectedExpire := 20 * time.Second
	expectedMaxConcurrent := int64(5)
	expectedDnstap := dnstap.Dnstap{}

	f, err := NewWithConfig(ForwardConfig{
		From:             "test",
		To:               []string{"1.2.3.4:3053", "tls://4.5.6.7"},
		Except:           []string{"FOO.com", "example.com"},
		MaxFails:         &expectedMaxFails,
		HealthCheck:      &expectedHealthCheck,
		HealthCheckNoRec: true,
		ForceTCP:         true,
		PreferUDP:        true,
		TLSConfig:        &tls.Config{NextProtos: []string{"some-proto"}},
		TLSServerName:    expectedServerName,
		Expire:           &expectedExpire,
		MaxConcurrent:    &expectedMaxConcurrent,
		TapPlugin:        &expectedDnstap,
	})
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	if f.from != "test." {
		t.Fatalf("Expected from to be %s, got: %s", "test.", f.from)
	}

	if len(f.proxies) != 2 {
		t.Fatalf("Expected proxies to have len of %d, got: %d", 2, len(f.proxies))
	}

	if f.proxies[0].addr != "1.2.3.4:3053" {
		t.Fatalf("Expected proxy to have addr of %s, got: %s", "1.2.3.4:3053", f.proxies[0].addr)
	}

	if f.proxies[1].addr != "4.5.6.7:853" {
		t.Fatalf("Expected proxy to have addr of %s, got: %s", "4.5.6.7:853", f.proxies[1].addr)
	}

	if !reflect.DeepEqual(f.ignored, expectedExcept) {
		t.Fatalf("Expected ignored to consist of %#v, got: %#v", expectedExcept, f.ignored)
	}

	if f.maxfails != 5 {
		t.Fatalf("Expected maxfails to be %d, got: %d", expectedMaxFails, f.maxfails)
	}

	if f.hcInterval != 5*time.Second {
		t.Fatalf("Expected hcInterval to be %s, got: %s", expectedHealthCheck, f.hcInterval)
	}

	if f.opts.hcRecursionDesired {
		t.Fatalf("Expected hcRecursionDesired to be false")
	}

	if !f.opts.forceTCP {
		t.Fatalf("Expected forceTCP to be true")
	}

	if !f.opts.preferUDP {
		t.Fatalf("Expected preferUDP to be true")
	}

	if len(f.tlsConfig.NextProtos) != 1 || f.tlsConfig.NextProtos[0] != "some-proto" {
		t.Fatalf("Expected tlsConfig to have NextProtos to consist of %s, got: %s", "some-proto", f.tlsConfig.NextProtos)
	}

	if f.tlsConfig.ServerName != expectedServerName {
		t.Fatalf("Expected tlsConfig to have ServerName to be %s, got: %s", expectedServerName, f.tlsConfig.ServerName)
	}

	if f.tlsServerName != "test" {
		t.Fatalf("Expected tlsSeverName to be %s, got: %s", expectedServerName, f.tlsServerName)
	}

	if f.expire != 20*time.Second {
		t.Fatalf("Expected expire to be %s, got: %s", expectedExpire, f.expire)
	}

	if f.ErrLimitExceeded == nil || f.ErrLimitExceeded.Error() != "concurrent queries exceeded maximum 5" {
		t.Fatalf("Expected ErrLimitExceeded to be %s, got: %s", "concurrent queries exceeded maximum 5", f.ErrLimitExceeded)
	}

	if f.maxConcurrent != 5 {
		t.Fatalf("Expected maxConcurrent to be %d, got: %d", 5, f.maxConcurrent)
	}

	if fmt.Sprintf("%T", f.tlsConfig.ClientSessionCache) != "*tls.lruSessionCache" {
		t.Fatalf("Expected tlsConfig.ClientSessionCache to be type %s, got: %T", "*tls.lruSessionCache", f.tlsConfig.ClientSessionCache)
	}

	if f.proxies[0].transport.expire != f.expire {
		t.Fatalf("Expected proxy.transport.expire to be %s, got: %s", f.expire, f.proxies[0].transport.expire)
	}

	if f.proxies[1].transport.expire != f.expire {
		t.Fatalf("Expected proxy.transport.expire to be %s, got: %s", f.expire, f.proxies[1].transport.expire)
	}

	if f.proxies[0].health.GetRecursionDesired() != f.opts.hcRecursionDesired {
		t.Fatalf("Expected proxy.health.GetRecursionDesired to be %t, got: %t", f.opts.hcRecursionDesired, f.proxies[0].health.GetRecursionDesired())
	}

	if f.proxies[1].health.GetRecursionDesired() != f.opts.hcRecursionDesired {
		t.Fatalf("Expected proxy.health.GetRecursionDesired to be %t, got: %t", f.opts.hcRecursionDesired, f.proxies[1].health.GetRecursionDesired())
	}

	if f.proxies[0].transport.tlsConfig == f.tlsConfig {
		t.Fatalf("Expected proxy.transport.tlsConfig to be nil, got: %#v", f.proxies[0].transport.tlsConfig)
	}

	if f.proxies[1].transport.tlsConfig != f.tlsConfig {
		t.Fatalf("Expected proxy.transport.tlsConfig to be %#v, got: %#v", f.tlsConfig, f.proxies[1].transport.tlsConfig)
	}

	if f.tapPlugin != &expectedDnstap {
		t.Fatalf("Expcted tapPlugin to be %p, got: %p", &expectedDnstap, f.tapPlugin)
	}
}

func TestNewWithConfigNegativeHealthCheck(t *testing.T) {
	healthCheck, _ := time.ParseDuration("-5s")

	_, err := NewWithConfig(ForwardConfig{
		To:          []string{"1.2.3.4:3053", "4.5.6.7"},
		HealthCheck: &healthCheck,
	})
	if err == nil || err.Error() != "health_check can't be negative: -5s" {
		t.Fatalf("Expected error to be %s, got: %s", "health_check can't be negative: -5s", err)
	}
}

func TestNewWithConfigNegativeExpire(t *testing.T) {
	expire, _ := time.ParseDuration("-5s")

	_, err := NewWithConfig(ForwardConfig{
		To:     []string{"1.2.3.4:3053", "4.5.6.7"},
		Expire: &expire,
	})
	if err == nil || err.Error() != "expire can't be negative: -5s" {
		t.Fatalf("Expected error to be %s, got: %s", "expire can't be negative: -5s", err)
	}
}

func TestNewWithConfigNegativeMaxConcurrent(t *testing.T) {
	maxConcurrent := int64(-5)

	_, err := NewWithConfig(ForwardConfig{
		To:            []string{"1.2.3.4:3053", "4.5.6.7"},
		MaxConcurrent: &maxConcurrent,
	})
	if err == nil || err.Error() != "max_concurrent can't be negative: -5" {
		t.Fatalf("Expected error to be %s, got: %s", "max_concurrent can't be negative: -5", err)
	}
}

func TestNewWithConfigPolicy(t *testing.T) {
	config := ForwardConfig{
		To: []string{"1.2.3.4:3053", "4.5.6.7"},
	}

	config.Policy = "random"
	f, err := NewWithConfig(config)
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	if _, ok := f.p.(*random); !ok {
		t.Fatalf("Expect p to be of type %s, got: %T", "random", f.p)
	}

	config.Policy = "round_robin"
	f, err = NewWithConfig(config)
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	if _, ok := f.p.(*roundRobin); !ok {
		t.Fatalf("Expect p to be of type %s, got: %T", "roundRobin", f.p)
	}

	config.Policy = "sequential"
	f, err = NewWithConfig(config)
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	if _, ok := f.p.(*sequential); !ok {
		t.Fatalf("Expect p to be of type %s, got: %T", "sequential", f.p)
	}

	config.Policy = "invalid_policy"
	_, err = NewWithConfig(config)
	if err == nil {
		t.Fatalf("Expected error %s, got: %s", "unknown policy 'invalid_policy'", err)
	}
}

func TestNewWithConfigServerNameDefault(t *testing.T) {
	f, err := NewWithConfig(ForwardConfig{
		To:        []string{"1.2.3.4"},
		TLSConfig: &tls.Config{ServerName: "some-server-name"},
	})
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	if f.tlsConfig.ServerName != "some-server-name" {
		t.Fatalf("Expect tlsConfig.ServerName to be %s, got: %s", "some-server-name", f.tlsConfig.ServerName)
	}
}

func TestNewWithConfigWithDefaults(t *testing.T) {
	f, err := NewWithConfig(ForwardConfig{
		To: []string{"1.2.3.4"},
	})
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	if f.from != "." {
		t.Fatalf("Expected from to be %s, got: %s", ".", f.from)
	}

	if f.ignored != nil {
		t.Fatalf("Expected ignored to be nil but was %#v", f.ignored)
	}

	if f.maxfails != 2 {
		t.Fatalf("Expected maxfails to be %d, got: %d", 2, f.maxfails)
	}

	if f.hcInterval != 500*time.Millisecond {
		t.Fatalf("Expected hcInterval to be %s, got: %s", 500*time.Millisecond, f.hcInterval)
	}

	if !f.opts.hcRecursionDesired {
		t.Fatalf("Expected hcRecursionDesired to be true")
	}

	if f.opts.forceTCP {
		t.Fatalf("Expected forceTCP to be false")
	}

	if f.opts.preferUDP {
		t.Fatalf("Expected preferUDP to be false")
	}

	if f.tlsConfig == nil {
		t.Fatalf("Expected tlsConfig to be non nil")
	}

	if f.tlsServerName != "" {
		t.Fatalf("Expected tlsServerName to be empty")
	}

	if f.expire != defaultExpire {
		t.Fatalf("Expected expire to be %s, got: %s", defaultExpire, f.expire)
	}

	if f.ErrLimitExceeded != nil {
		t.Fatalf("Expected ErrLimitExceeded to be nil")
	}

	if f.maxConcurrent != 0 {
		t.Fatalf("Expected maxConcurrent to be %d, got: %d", 0, f.maxConcurrent)
	}

	if _, ok := f.p.(*random); !ok {
		t.Fatalf("Expect p to be of type %s, got: %T", "random", f.p)
	}
}
