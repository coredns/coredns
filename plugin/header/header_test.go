package header

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestHeaderResponseRules(t *testing.T) {
	wr := dnstest.NewRecorder(&test.ResponseWriter{})
	next := plugin.HandlerFunc(func(ctx context.Context, writer dns.ResponseWriter, msg *dns.Msg) (int, error) {
		writer.WriteMsg(msg)
		return dns.RcodeSuccess, nil
	})

	tests := []struct {
		handler  plugin.Handler
		got      func(msg *dns.Msg) bool
		expected bool
	}{
		{
			handler: Header{
				ResponseRules: []Rule{{Flag: recursionAvailable, State: true}},
				Next:          next,
			},
			got: func(msg *dns.Msg) bool {
				return msg.RecursionAvailable
			},
			expected: true,
		},
		{
			handler: Header{
				ResponseRules: []Rule{{Flag: recursionAvailable, State: false}},
				Next:          next,
			},
			got: func(msg *dns.Msg) bool {
				return msg.RecursionAvailable
			},
			expected: false,
		},
		{
			handler: Header{
				ResponseRules: []Rule{{Flag: recursionDesired, State: true}},
				Next:          next,
			},
			got: func(msg *dns.Msg) bool {
				return msg.RecursionDesired
			},
			expected: true,
		},
		{
			handler: Header{
				ResponseRules: []Rule{{Flag: authoritative, State: true}},
				Next:          next,
			},
			got: func(msg *dns.Msg) bool {
				return msg.Authoritative
			},
			expected: true,
		},
	}

	for i, test := range tests {
		m := new(dns.Msg)

		_, err := test.handler.ServeDNS(context.TODO(), wr, m)
		if err != nil {
			t.Errorf("Test %d: Expected no error, but got %s", i, err)
			continue
		}

		if test.got(m) != test.expected {
			t.Errorf("Test %d: Expected flag state=%t, but got %t", i, test.expected, test.got(m))
			continue
		}
	}
}

func TestHeaderQueryRules(t *testing.T) {
	wr := dnstest.NewRecorder(&test.ResponseWriter{})
	next := plugin.HandlerFunc(func(ctx context.Context, writer dns.ResponseWriter, msg *dns.Msg) (int, error) {
		writer.WriteMsg(msg)
		return dns.RcodeSuccess, nil
	})

	tests := []struct {
		handler  plugin.Handler
		got      func(msg *dns.Msg) bool
		expected bool
	}{
		{
			handler: Header{
				QueryRules: []Rule{{Flag: recursionAvailable, State: true}},
				Next:       next,
			},
			got: func(msg *dns.Msg) bool {
				return msg.RecursionAvailable
			},
			expected: true,
		},
		{
			handler: Header{
				QueryRules: []Rule{{Flag: recursionDesired, State: true}},
				Next:       next,
			},
			got: func(msg *dns.Msg) bool {
				return msg.RecursionDesired
			},
			expected: true,
		},
		{
			handler: Header{
				QueryRules: []Rule{{Flag: recursionDesired, State: false}},
				Next:       next,
			},
			got: func(msg *dns.Msg) bool {
				return msg.RecursionDesired
			},
			expected: false,
		},
		{
			handler: Header{
				QueryRules: []Rule{{Flag: authoritative, State: true}},
				Next:       next,
			},
			got: func(msg *dns.Msg) bool {
				return msg.Authoritative
			},
			expected: true,
		},
	}

	for i, tc := range tests {
		m := new(dns.Msg)

		_, err := tc.handler.ServeDNS(context.TODO(), wr, m)
		if err != nil {
			t.Errorf("Test %d: Expected no error, but got %s", i, err)
			continue
		}

		if tc.got(m) != tc.expected {
			t.Errorf("Test %d: Expected flag state=%t, but got %t", i, tc.expected, tc.got(m))
			continue
		}
	}
}
