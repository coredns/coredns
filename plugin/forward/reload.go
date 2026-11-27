package forward

import (
	"errors"
	"slices"
	"time"

	"github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"
)

// startReload starts a goroutine that periodically re-reads the file based TO
// entries (e.g. /etc/resolv.conf) and updates the upstreams when they changed.
func (f *Forward) startReload() {
	if f.reloadInterval == 0 || !slices.ContainsFunc(f.toEntries, func(e toEntry) bool { return e.file != "" }) {
		return
	}
	f.reloadStop = make(chan struct{})
	go func(stop <-chan struct{}) {
		tick := time.NewTicker(f.reloadInterval)
		defer tick.Stop()
		var lastErr string
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				err := f.reloadFiles()
				if err != nil && err.Error() != lastErr {
					log.Warningf("Failed to reload upstreams for %q, keeping the current ones: %s", f.from, err)
				}
				lastErr = ""
				if err != nil {
					lastErr = err.Error()
				}
			}
		}
	}(f.reloadStop)
}

func (f *Forward) stopReload() {
	f.reloadOnce.Do(func() {
		if f.reloadStop != nil {
			close(f.reloadStop)
		}
	})
}

// reloadFiles re-reads all file based TO entries. If the resulting list of
// upstreams differs from the current one, new proxies are started for added
// upstreams and proxies of removed upstreams are stopped. Proxies of upstreams
// that are still present are kept, together with their health state.
func (f *Forward) reloadFiles() error {
	entries := slices.Clone(f.toEntries)
	for i, e := range entries {
		if e.file == "" {
			continue
		}
		addrs, err := parse.HostPortOrFile(e.file)
		if err != nil && !errors.Is(err, parse.ErrNoNameservers) {
			return err
		}
		entries[i].addrs = addrs
	}

	hosts := dedupAddrs(entries)
	if len(hosts) == 0 {
		return errors.New("no valid upstream addresses found")
	}

	f.mu.RLock()
	old, oldHosts := f.proxies, f.hosts
	f.mu.RUnlock()
	if len(old) != len(oldHosts) {
		return errors.New("upstreams were not set up from the configuration")
	}
	current := make(map[string]*proxy.Proxy, len(old))
	for i, p := range old {
		current[dedupKey(oldHosts[i])] = p
	}

	proxies := make([]*proxy.Proxy, 0, len(hosts))
	var added []*proxy.Proxy
	for _, h := range hosts {
		key := dedupKey(h)
		if p, ok := current[key]; ok {
			proxies = append(proxies, p)
			delete(current, key)
			continue
		}
		trans, addr := parse.Transport(h)
		if trans != transport.DNS {
			// Only file entries change and those only produce plain DNS upstreams.
			return errors.New("unexpected new upstream " + h)
		}
		p := proxy.NewProxy("forward", addr, trans)
		f.configureProxy(p, trans)
		proxies = append(proxies, p)
		added = append(added, p)
	}

	if len(added) == 0 && len(current) == 0 && slices.Equal(proxies, old) {
		return nil
	}

	for _, p := range added {
		f.startProxy(p)
	}
	f.mu.Lock()
	f.proxies, f.hosts = proxies, hosts
	f.mu.Unlock()
	// In-flight queries may still hold a removed proxy, its connections are
	// cleaned up once it is garbage collected.
	for _, p := range current {
		p.Stop()
	}

	log.Infof("Upstreams for %q changed to %v", f.from, hosts)
	return nil
}
