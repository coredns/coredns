package forwardcrd

import (
	"sync"

	"github.com/coredns/coredns/plugin"
)

// PluginInstanceMap represents a map of zones to coredns plugin instances that
// is thread-safe. It enables the forwardcrd plugin to save the state of
// which plugin instances should be delegated to for a given zone.
type PluginInstanceMap struct {
	mutex          *sync.RWMutex
	zonesToPlugins map[string]lifecyclePluginHandler
	keyToZones     map[string]string
}

// NewPluginInstanceMap returns a new instance of PluginInstanceMap.
func NewPluginInstanceMap() *PluginInstanceMap {
	return &PluginInstanceMap{
		mutex:          &sync.RWMutex{},
		zonesToPlugins: make(map[string]lifecyclePluginHandler),
		keyToZones:     make(map[string]string),
	}
}

// Upsert adds or updates the map with a zone to plugin handler mapping. If the
// same key is provided it will overwrite the old zone for that key with the
// new zone. Returns the plugin instance and true if the upsert was an update
// operation and not a create operation.
func (p *PluginInstanceMap) Upsert(key, zone string, handler lifecyclePluginHandler) (lifecyclePluginHandler, bool) {
	var isUpdate bool
	var oldPlugin lifecyclePluginHandler
	p.mutex.Lock()
	normalizedZone := plugin.Host(zone).NormalizeExact()[0] // there can only be one here, won't work with non-octet reverse
	oldZone, ok := p.keyToZones[key]
	if ok {
		oldPlugin = p.zonesToPlugins[oldZone]
		isUpdate = true
		delete(p.zonesToPlugins, oldZone)
	}

	p.keyToZones[key] = normalizedZone
	p.zonesToPlugins[normalizedZone] = handler
	p.mutex.Unlock()
	return oldPlugin, isUpdate
}

// Get gets the plugin handler provided a zone name. It will return true if the
// plugin handler exists and false if it does not exist.
func (p *PluginInstanceMap) Get(zone string) (lifecyclePluginHandler, bool) {
	p.mutex.RLock()
	normalizedZone := plugin.Host(zone).NormalizeExact()[0] // there can only be one here, won't work with non-octet reverse
	handler, ok := p.zonesToPlugins[normalizedZone]
	p.mutex.RUnlock()
	return handler, ok
}

// List lists all the plugin instances in the map.
func (p *PluginInstanceMap) List() []lifecyclePluginHandler {
	p.mutex.RLock()
	plugins := make([]lifecyclePluginHandler, len(p.zonesToPlugins))
	var i int
	for _, v := range p.zonesToPlugins {
		plugins[i] = v
		i++
	}
	p.mutex.RUnlock()
	return plugins
}

// Delete deletes the zone and plugin handler from the map. Returns the plugin
// instance that was deleted, useful for shutting down. Returns nil if no
// plugin was found.
func (p *PluginInstanceMap) Delete(key string) lifecyclePluginHandler {
	p.mutex.Lock()
	zone := p.keyToZones[key]
	plugin := p.zonesToPlugins[zone]
	delete(p.zonesToPlugins, zone)
	delete(p.keyToZones, key)
	p.mutex.Unlock()
	return plugin
}
