// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hosts

import (
	"reflect"
	"strings"
	"testing"
)

type staticHostEntry struct {
	in  string
	out []string
}

var lookupStaticHostTests = []struct {
	name string
	ents []staticHostEntry
}{
	{
		"testdata/hosts",
		[]staticHostEntry{
			{"odin", []string{"127.0.0.2", "127.0.0.3", "::2"}},
			{"thor", []string{"127.1.1.1"}},
			{"ullr", []string{"127.1.1.2"}},
			{"ullrhost", []string{"127.1.1.2"}},
			{"localhost", []string{"fe80::1"}},
		},
	},
	{
		"testdata/singleline-hosts", // see golang.org/issue/6646
		[]staticHostEntry{
			{"odin", []string{"127.0.0.2"}},
		},
	},
	{
		"testdata/ipv4-hosts", // see golang.org/issue/8996
		[]staticHostEntry{
			{"localhost", []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"}},
			{"localhost.localdomain", []string{"127.0.0.3"}},
		},
	},
	{
		"testdata/ipv6-hosts", // see golang.org/issue/8996
		[]staticHostEntry{
			{"localhost", []string{"::1", "fe80::1", "fe80::2", "fe80::3"}},
			{"localhost.localdomain", []string{"fe80::3"}},
		},
	},
	{
		"testdata/case-hosts", // see golang.org/issue/12806
		[]staticHostEntry{
			{"PreserveMe", []string{"127.0.0.1", "::1"}},
			{"PreserveMe.local", []string{"127.0.0.1", "::1"}},
		},
	},
}

func TestLookupStaticHost(t *testing.T) {

	for _, tt := range lookupStaticHostTests {
		h := &Hostsfile{path: tt.name}
		for _, ent := range tt.ents {
			testStaticHost(t, ent, h)
		}
	}
}

func testStaticHost(t *testing.T, ent staticHostEntry, h *Hostsfile) {
	ins := []string{ent.in, absDomainName([]byte(ent.in)), strings.ToLower(ent.in), strings.ToUpper(ent.in)}
	for _, in := range ins {
		addrs := h.LookupStaticHost(in)
		if !reflect.DeepEqual(addrs, ent.out) {
			t.Errorf("%s, lookupStaticHost(%s) = %v; want %v", h.path, in, addrs, ent.out)
		}
	}
}

var lookupStaticAddrTests = []struct {
	name string
	ents []staticHostEntry
}{
	{
		"testdata/hosts",
		[]staticHostEntry{
			{"255.255.255.255", []string{"broadcasthost"}},
			{"127.0.0.2", []string{"odin"}},
			{"127.0.0.3", []string{"odin"}},
			{"::2", []string{"odin"}},
			{"127.1.1.1", []string{"thor"}},
			{"127.1.1.2", []string{"ullr", "ullrhost"}},
			{"fe80::1", []string{"localhost"}},
		},
	},
	{
		"testdata/singleline-hosts", // see golang.org/issue/6646
		[]staticHostEntry{
			{"127.0.0.2", []string{"odin"}},
		},
	},
	{
		"testdata/ipv4-hosts", // see golang.org/issue/8996
		[]staticHostEntry{
			{"127.0.0.1", []string{"localhost"}},
			{"127.0.0.2", []string{"localhost"}},
			{"127.0.0.3", []string{"localhost", "localhost.localdomain"}},
		},
	},
	{
		"testdata/ipv6-hosts", // see golang.org/issue/8996
		[]staticHostEntry{
			{"::1", []string{"localhost"}},
			{"fe80::1", []string{"localhost"}},
			{"fe80::2", []string{"localhost"}},
			{"fe80::3", []string{"localhost", "localhost.localdomain"}},
		},
	},
	{
		"testdata/case-hosts", // see golang.org/issue/12806
		[]staticHostEntry{
			{"127.0.0.1", []string{"PreserveMe", "PreserveMe.local"}},
			{"::1", []string{"PreserveMe", "PreserveMe.local"}},
		},
	},
}

func TestLookupStaticAddr(t *testing.T) {
	for _, tt := range lookupStaticAddrTests {
		h := &Hostsfile{path: tt.name}
		for _, ent := range tt.ents {
			testStaticAddr(t, ent, h)
		}
	}
}

func testStaticAddr(t *testing.T, ent staticHostEntry, h *Hostsfile) {
	hosts := h.LookupStaticAddr(ent.in)
	for i := range ent.out {
		ent.out[i] = absDomainName([]byte(ent.out[i]))
	}
	if !reflect.DeepEqual(hosts, ent.out) {
		t.Errorf("%s, lookupStaticAddr(%s) = %v; want %v", h.path, ent.in, hosts, h)
	}
}

func TestHostCacheModification(t *testing.T) {
	// Ensure that programs can't modify the internals of the host cache.
	// See https://github.com/golang/go/issues/14212.

	h := &Hostsfile{path: "testdata/ipv4-hosts"}
	ent := staticHostEntry{"localhost", []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"}}
	testStaticHost(t, ent, h)
	// Modify the addresses return by lookupStaticHost.
	addrs := h.LookupStaticHost(ent.in)
	for i := range addrs {
		addrs[i] += "junk"
	}
	testStaticHost(t, ent, h)

	h = &Hostsfile{path: "testdata/ipv6-hosts"}
	ent = staticHostEntry{"::1", []string{"localhost"}}
	testStaticAddr(t, ent, h)
	// Modify the hosts return by lookupStaticAddr.
	hosts := h.LookupStaticAddr(ent.in)
	for i := range hosts {
		hosts[i] += "junk"
	}
	testStaticAddr(t, ent, h)
}
