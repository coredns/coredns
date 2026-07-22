package dnsserver

import (
	"testing"

	"github.com/coredns/caddy/caddyfile"
)

func TestHandler(t *testing.T) {
	tp := testPlugin{}
	c := testConfig("dns", tp)
	if _, err := NewServer("127.0.0.1:53", []*Config{c}); err != nil {
		t.Errorf("Expected no error for NewServer, got %s", err)
	}
	if h := c.Handler("local"); h != tp {
		t.Errorf("Expected testPlugin from Handler, got %T", h)
	}
	if h := c.Handler("nothing"); h != nil {
		t.Errorf("Expected nil from Handler, got %T", h)
	}
}

func TestHandlers(t *testing.T) {
	tp := testPlugin{}
	c := testConfig("dns", tp)
	if _, err := NewServer("127.0.0.1:53", []*Config{c}); err != nil {
		t.Errorf("Expected no error for NewServer, got %s", err)
	}
	hs := c.Handlers()
	if len(hs) != 1 || hs[0] != tp {
		t.Errorf("Expected [testPlugin] from Handlers, got %v", hs)
	}
}

func TestGroupingServers(t *testing.T) {
	for i, test := range []struct {
		configs        []*Config
		expectedGroups []string
		failing        bool
	}{
		// single config -> one group
		{configs: []*Config{
			{Transport: "dns", Zone: ".", Port: "53", ListenHosts: []string{""}},
		},
			expectedGroups: []string{"dns://:53"},
			failing:        false},

		// 2 configs on different port -> 2 groups
		{configs: []*Config{
			{Transport: "dns", Zone: ".", Port: "53", ListenHosts: []string{""}},
			{Transport: "dns", Zone: ".", Port: "54", ListenHosts: []string{""}},
		},
			expectedGroups: []string{"dns://:53", "dns://:54"},
			failing:        false},

		// 2 configs on same port, both not using bind, diff zones -> 1 group
		{configs: []*Config{
			{Transport: "dns", Zone: ".", Port: "53", ListenHosts: []string{""}},
			{Transport: "dns", Zone: "com.", Port: "53", ListenHosts: []string{""}},
		},
			expectedGroups: []string{"dns://:53"},
			failing:        false},

		// 2 configs on same port, one addressed - one not using bind, diff zones -> 1 group
		{configs: []*Config{
			{Transport: "dns", Zone: ".", Port: "53", ListenHosts: []string{"127.0.0.1"}},
			{Transport: "dns", Zone: ".", Port: "54", ListenHosts: []string{""}},
		},
			expectedGroups: []string{"dns://127.0.0.1:53", "dns://:54"},
			failing:        false},

		// 2 configs on diff ports, 3 different address, diff zones -> 3 group
		{configs: []*Config{
			{Transport: "dns", Zone: ".", Port: "53", ListenHosts: []string{"127.0.0.1", "::1"}},
			{Transport: "dns", Zone: ".", Port: "54", ListenHosts: []string{""}}},
			expectedGroups: []string{"dns://127.0.0.1:53", "dns://[::1]:53", "dns://:54"},
			failing:        false},

		// 2 configs on same port, same address, diff zones -> 1 group
		{configs: []*Config{
			{Transport: "dns", Zone: ".", Port: "53", ListenHosts: []string{"127.0.0.1", "::1"}},
			{Transport: "dns", Zone: "com.", Port: "53", ListenHosts: []string{"127.0.0.1", "::1"}},
		},
			expectedGroups: []string{"dns://127.0.0.1:53", "dns://[::1]:53"},
			failing:        false},

		// 2 configs on same port, total 2 diff addresses, diff zones -> 2 groups
		{configs: []*Config{
			{Transport: "dns", Zone: ".", Port: "53", ListenHosts: []string{"127.0.0.1"}},
			{Transport: "dns", Zone: "com.", Port: "53", ListenHosts: []string{"::1"}},
		},
			expectedGroups: []string{"dns://127.0.0.1:53", "dns://[::1]:53"},
			failing:        false},

		// 2 configs on same port, total 3 diff addresses, diff zones -> 3 groups
		{configs: []*Config{
			{Transport: "dns", Zone: ".", Port: "53", ListenHosts: []string{"127.0.0.1", "::1"}},
			{Transport: "dns", Zone: "com.", Port: "53", ListenHosts: []string{""}}},
			expectedGroups: []string{"dns://127.0.0.1:53", "dns://[::1]:53", "dns://:53"},
			failing:        false},
	} {
		groups, err := groupConfigsByListenAddr(test.configs)
		if err != nil {
			if !test.failing {
				t.Fatalf("Test %d, expected no errors, but got: %v", i, err)
			}
			continue
		}
		if test.failing {
			t.Fatalf("Test %d, expected to failed but did not, returned values", i)
		}
		if len(groups) != len(test.expectedGroups) {
			t.Errorf("Test %d : expected the group's size to be %d, was %d", i, len(test.expectedGroups), len(groups))
			continue
		}
		for _, v := range test.expectedGroups {
			if _, ok := groups[v]; !ok {
				t.Errorf("Test %d : expected value %v to be in the group, was not", i, v)
			}
		}
	}
}

func TestInspectServerBlocks(t *testing.T) {
	tests := []struct {
		name                 string
		serverBlocks         []caddyfile.ServerBlock
		expectedServerBlocks []caddyfile.ServerBlock
		expectedConfigsLen   int
		expectedZoneAddrs    map[string]zoneAddr
		wantErr              bool
	}{
		{
			name: "simple dns",
			serverBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"example.org"}},
			},
			expectedServerBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"dns://example.org.:53"}},
			},
			expectedConfigsLen: 1,
			expectedZoneAddrs: map[string]zoneAddr{
				"dns://example.org.:53": {Zone: "example.org.", Port: "53", Transport: "dns"},
			},
		},
		{
			name: "uppercase dns",
			serverBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"EXAMPLE.ORG"}},
			},
			expectedServerBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"dns://example.org.:53"}},
			},
			expectedConfigsLen: 1,
			expectedZoneAddrs: map[string]zoneAddr{
				"dns://example.org.:53": {Zone: "example.org.", Port: "53", Transport: "dns"},
			},
		},
		{
			name: "dns with port",
			serverBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"example.org:1053"}},
			},
			expectedServerBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"dns://example.org.:1053"}},
			},
			expectedConfigsLen: 1,
			expectedZoneAddrs: map[string]zoneAddr{
				"dns://example.org.:1053": {Zone: "example.org.", Port: "1053", Transport: "dns"},
			},
		},
		{
			name: "tls",
			serverBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"tls://example.org"}},
			},
			expectedServerBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"tls://example.org.:853"}},
			},
			expectedConfigsLen: 1,
			expectedZoneAddrs: map[string]zoneAddr{
				"tls://example.org.:853": {Zone: "example.org.", Port: "853", Transport: "tls"},
			},
		},
		{
			name: "quic",
			serverBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"quic://example.org"}},
			},
			expectedServerBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"quic://example.org.:853"}},
			},
			expectedConfigsLen: 1,
			expectedZoneAddrs: map[string]zoneAddr{
				"quic://example.org.:853": {Zone: "example.org.", Port: "853", Transport: "quic"},
			},
		},
		{
			name: "grpc",
			serverBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"grpc://example.org"}},
			},
			expectedServerBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"grpc://example.org.:443"}},
			},
			expectedConfigsLen: 1,
			expectedZoneAddrs: map[string]zoneAddr{
				"grpc://example.org.:443": {Zone: "example.org.", Port: "443", Transport: "grpc"},
			},
		},
		{
			name: "https",
			serverBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"https://example.org."}},
			},
			expectedServerBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"https://example.org.:443"}},
			},
			expectedConfigsLen: 1,
			expectedZoneAddrs: map[string]zoneAddr{
				"https://example.org.:443": {Zone: "example.org.", Port: "443", Transport: "https"},
			},
		},
		{
			name: "multiple hosts same key",
			serverBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"example.org,example.com:1053"}},
			},
			expectedServerBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"dns://example.org,example.com.:1053"}},
			},
			expectedConfigsLen: 1,
			expectedZoneAddrs: map[string]zoneAddr{
				"dns://example.org,example.com.:1053": {Zone: "example.org,example.com.", Port: "1053", Transport: "dns"},
			},
		},
		{
			name: "multiple keys",
			serverBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"example.org", "example.com:1053"}},
			},
			expectedServerBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"dns://example.org.:53", "dns://example.com.:1053"}},
			},
			expectedConfigsLen: 2,
			expectedZoneAddrs: map[string]zoneAddr{
				"dns://example.org.:53":   {Zone: "example.org.", Port: "53", Transport: "dns"},
				"dns://example.com.:1053": {Zone: "example.com.", Port: "1053", Transport: "dns"},
			},
		},
		{
			name: "fqdn input",
			serverBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"example.org."}},
			},
			expectedServerBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"dns://example.org.:53"}},
			},
			expectedConfigsLen: 1,
			expectedZoneAddrs: map[string]zoneAddr{
				"dns://example.org.:53": {Zone: "example.org.", Port: "53", Transport: "dns"},
			},
		},
		{
			name: "multiple server blocks",
			serverBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"example.org"}},
				{Keys: []string{"sub.example.org:1054"}},
			},
			expectedServerBlocks: []caddyfile.ServerBlock{
				{Keys: []string{"dns://example.org.:53"}},
				{Keys: []string{"dns://sub.example.org.:1054"}},
			},
			expectedConfigsLen: 2,
			expectedZoneAddrs: map[string]zoneAddr{
				"dns://example.org.:53":       {Zone: "example.org.", Port: "53", Transport: "dns"},
				"dns://sub.example.org.:1054": {Zone: "sub.example.org.", Port: "1054", Transport: "dns"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newContext(nil).(*dnsContext)
			processedBlocks, err := ctx.InspectServerBlocks("TestInspectServerBlocks", tc.serverBlocks)

			if (err != nil) != tc.wantErr {
				t.Fatalf("InspectServerBlocks() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			if len(processedBlocks) != len(tc.expectedServerBlocks) {
				t.Fatalf("Expected %d processed blocks, got %d", len(tc.expectedServerBlocks), len(processedBlocks))
			}

			for i, block := range processedBlocks {
				expectedBlock := tc.expectedServerBlocks[i]
				if len(block.Keys) != len(expectedBlock.Keys) {
					t.Errorf("Block %d: expected %d keys, got %d. Expected: %v, Got: %v", i, len(expectedBlock.Keys), len(block.Keys), expectedBlock.Keys, block.Keys)
					continue
				}
				for j, key := range block.Keys {
					if key != expectedBlock.Keys[j] {
						t.Errorf("Block %d, Key %d: expected key '%s', got '%s'", i, j, expectedBlock.Keys[j], key)
					}
				}
			}

			if len(ctx.configs) != tc.expectedConfigsLen {
				t.Errorf("Expected %d configs to be created, got %d", tc.expectedConfigsLen, len(ctx.configs))
			}

			if tc.expectedZoneAddrs != nil {
				configIndex := 0
				for ib := range processedBlocks {
					for ik, key := range processedBlocks[ib].Keys {
						if configIndex >= len(ctx.configs) {
							t.Fatalf("Not enough configs stored, expected at least %d, processed block %d key %d", configIndex+1, ib, ik)
						}
						cfg := ctx.configs[configIndex]
						expectedZa, ok := tc.expectedZoneAddrs[key]
						if !ok {
							t.Errorf("No expected zoneAddr for processed key '%s'", key)
							continue
						}

						if cfg.Zone != expectedZa.Zone {
							t.Errorf("Config for key '%s': expected Zone '%s', got '%s'", key, expectedZa.Zone, cfg.Zone)
						}
						if cfg.Port != expectedZa.Port {
							t.Errorf("Config for key '%s': expected Port '%s', got '%s'", key, expectedZa.Port, cfg.Port)
						}
						if cfg.Transport != expectedZa.Transport {
							t.Errorf("Config for key '%s': expected Transport '%s', got '%s'", key, expectedZa.Transport, cfg.Transport)
						}
						if len(cfg.ListenHosts) != 1 || cfg.ListenHosts[0] != "" {
							t.Errorf("Config for key '%s': expected ListenHosts [''], got %v", key, cfg.ListenHosts)
						}
						configIndex++
					}
				}
			}
		})
	}
}
