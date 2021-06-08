package forwardcrd

import (
	"sync"
	"testing"

	"github.com/coredns/coredns/plugin/forward"
)

func TestPluginMap(t *testing.T) {
	pluginInstanceMap := NewPluginInstanceMap()

	zone1ForwardPlugin := forward.New()
	zone2ForwardPlugin := forward.New()

	// Testing concurrency to ensure map is thread-safe
	// i.e should run with `go test -race`
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		pluginInstanceMap.Upsert("default/some-dns-zone", "zone-1.test", zone1ForwardPlugin)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		pluginInstanceMap.Upsert("default/another-dns-zone", "zone-2.test", zone2ForwardPlugin)
		wg.Done()
	}()
	wg.Wait()

	if plugin, exists := pluginInstanceMap.Get("zone-1.test."); exists && plugin != zone1ForwardPlugin {
		t.Fatalf("Expected plugin instance map to get plugin with address: %p but was: %p", zone1ForwardPlugin, plugin)
	}

	if plugin, exists := pluginInstanceMap.Get("zone-2.test"); exists && plugin != zone2ForwardPlugin {
		t.Fatalf("Expected plugin instance map to get plugin with address: %p but was: %p", zone2ForwardPlugin, plugin)
	}

	if _, exists := pluginInstanceMap.Get("non-existant-zone.test"); exists {
		t.Fatal("Expected plugin instance map to not return a plugin")
	}

	// list

	plugins := pluginInstanceMap.List()
	if len(plugins) != 2 {
		t.Fatalf("Expected plugin instance map to have len %d, got: %d", 2, len(plugins))
	}

	if plugins[0] != zone1ForwardPlugin && plugins[0] != zone2ForwardPlugin {
		t.Fatalf("Expected plugin instance map to list plugin[0] with address: %p or %p but was: %p", zone1ForwardPlugin, zone2ForwardPlugin, plugins[0])
	}

	if plugins[1] != zone1ForwardPlugin && plugins[1] != zone2ForwardPlugin {
		t.Fatalf("Expected plugin instance map to list plugin[1] with address: %p or %p but was: %p", zone1ForwardPlugin, zone2ForwardPlugin, plugins[1])
	}

	// update record with the same key

	oldPlugin, update := pluginInstanceMap.Upsert("default/some-dns-zone", "new-zone-1.test", zone1ForwardPlugin)

	if !update {
		t.Fatalf("Expected Upsert to be an update")
	}

	if oldPlugin != zone1ForwardPlugin {
		t.Fatalf("Expected Upsert to return the old plugin %#v, got: %#v", zone1ForwardPlugin, oldPlugin)
	}

	if plugin, exists := pluginInstanceMap.Get("new-zone-1.test"); exists && plugin != zone1ForwardPlugin {
		t.Fatalf("Expected plugin instance map to get plugin with address: %p but was: %p", zone1ForwardPlugin, plugin)

	}
	if _, exists := pluginInstanceMap.Get("zone-1.test"); exists {
		t.Fatalf("Expected plugin instance map to not get plugin with zone: %s", "zone-1.test")
	}

	// delete record by key

	deletedPlugin := pluginInstanceMap.Delete("default/some-dns-zone")

	if _, exists := pluginInstanceMap.Get("new-zone-1.test"); exists {
		t.Fatalf("Expected plugin instance map to not get plugin with zone: %s", "new-zone-1.test")
	}

	if deletedPlugin == nil || deletedPlugin != zone1ForwardPlugin {
		t.Fatalf("Expected Delete to return the deleted plugin %#v, got: %#v", zone1ForwardPlugin, deletedPlugin)
	}
}
