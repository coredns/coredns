package dnsserver

import (
	"testing"
)

func TestHandler(t *testing.T) {
	tp := testPlugin{}
	c := testConfig("dns", tp)
	if _, err := NewServer("127.0.0.1:53", []*Config{c}); err != nil {
		t.Errorf("Expected no error for NewServer, got %s", err)
	}
	if h := c.Handler("testplugin"); h != tp {
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

		// 2 configs on same port, both undound, diff zones -> 1 group
		{configs: []*Config{
			{Transport: "dns", Zone: ".", Port: "53", ListenHosts: []string{""}},
			{Transport: "dns", Zone: "com.", Port: "53", ListenHosts: []string{""}},
		},
			expectedGroups: []string{"dns://:53"},
			failing:        false},

		// 2 configs on same port, one addressed - one unbound, diff zones -> 1 group
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
				t.Fatalf("test %d, expected no errors, but got: %v", i, err)
			}
			continue
		}
		if test.failing {
			t.Fatalf("test %d, expected to failed but did not, returned values", i)
		}
		if len(groups) != len(test.expectedGroups) {
			t.Errorf("test %d : expected the group's size to be %d, was %d", i, len(test.expectedGroups), len(groups))
			continue
		}
		for _, v := range test.expectedGroups {
			if _, ok := groups[v]; !ok {
				t.Errorf("test %d : expected value %v to be in the group, was not", i, v)

			}
		}
	}
}

func checkConfigvalues(t *testing.T, cfg *Config, zone string, hosts []string) {
	if cfg == nil {
		t.Errorf("expected config but got a nil pts")
	}
	if cfg.Zone != zone {
		t.Errorf("expected zone %v, but got %v", zone, cfg.Zone)
	}
	if len(cfg.ListenHosts) != len(hosts) {
		t.Errorf("expected count of listening host :  %v, but got %v", len(hosts), len(cfg.ListenHosts))
	}
	for i, h := range hosts {
		if cfg.ListenHosts[i] != h {
			t.Errorf("expected listening host %v, but got %v", h, cfg.ListenHosts[i])
		}
	}
}
func TestConfigPersistence(t *testing.T) {
	// verify that the right config is saved in the right place
	// verify we can now save configs with same zone
	ctx := &dnsContext{keysToConfigs: make(map[string]*Config)}

	test := []struct {
		blocindex int
		keyindex  int
		cfg       *Config
	}{
		{blocindex: 1, keyindex: 1, cfg: &Config{Zone: "one", ListenHosts: []string{"one", "two"}}},
		{blocindex: 1, keyindex: 2, cfg: &Config{Zone: "two", ListenHosts: []string{"one"}}},
		{blocindex: 2, keyindex: 1, cfg: &Config{Zone: "one", ListenHosts: []string{"one", "two"}}},
		{blocindex: 2, keyindex: 2, cfg: &Config{Zone: "one", ListenHosts: []string{"one"}}},
		{blocindex: 0, keyindex: 0, cfg: &Config{Zone: ".", ListenHosts: []string{""}}},
	}

	for _, tt := range test {
		ctx.saveConfig(tt.blocindex, tt.keyindex, tt.cfg)

	}

	if len(ctx.keysToConfigs) != len(test) {
		t.Fatalf("expected %v configs saved in context, got %v", len(test), len(ctx.keysToConfigs))
	}

	// now verify there is no mixup in the saved zones
	for i, tt := range test {
		cfg, ok := ctx.getConfig(tt.blocindex, tt.keyindex)
		if !ok {
			t.Fatalf("test %v, cannot retrieve the config from context", i)
		}
		checkConfigvalues(t, cfg, tt.cfg.Zone, tt.cfg.ListenHosts)
	}
}
