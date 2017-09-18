package file

import (
	"log"
	"os"
	"path"
	"sync"

	"github.com/fsnotify/fsnotify"
)

var watcher = newWatch()

type watch struct {
	w *fsnotify.Watcher
	d map[string]int
	sync.RWMutex
}

func newWatch() *watch {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		panic("plugin/file: failed to get fsnotify.Watcher: " + err.Error())
	}
	return &watch{w: w, d: make(map[string]int), RWMutex: sync.RWMutex{}}
}

func (w *watch) add(dir string) error {
	w.Lock()
	defer w.Unlock()
	if _, ok := w.d[dir]; ok {
		w.d[dir]++
		return nil
	}
	w.d[dir]++

	return w.w.Add(dir)
}

func (w *watch) remove(dir string) error {
	w.RLock()
	defer w.RUnlock()
	i, ok := w.d[dir]
	if !ok {
		return nil
	}
	i--
	if i == 0 {
		err := w.w.Remove(dir)
		delete(w.d, dir)
		return err
	}
	w.d[dir]--
	return nil
}

// Reload reloads a zone when it is changed on disk. If z.NoRoload is true, no reloading will be done.
func (z *Zone) Reload() error {
	if z.NoReload {
		return nil
	}

	if err := watcher.add(path.Dir(z.file)); err != nil {
		return err
	}

	go func() {
		for {
			select {
			case event := <-watcher.w.Events:
				if path.Clean(event.Name) == z.file {
					// You should move stuff into place!
					if event.Op&fsnotify.Create != fsnotify.Create {
						continue
					}

					reader, err := os.Open(z.file)
					if err != nil {
						log.Printf("[ERROR] Failed to open `%s' for `%s': %v", z.file, z.origin, err)
						continue
					}

					serial := z.SOASerialIfDefined()
					zone, err := Parse(reader, z.origin, z.file, serial)
					if err != nil {
						log.Printf("[INFO] Parsing zone `%s': %v", z.origin, err)
						continue
					}

					// copy elements we need
					z.reloadMu.Lock()
					z.Apex = zone.Apex
					z.Tree = zone.Tree
					z.reloadMu.Unlock()

					log.Printf("[INFO] Successfully reloaded zone `%s': serial: %d", z.origin, z.Apex.SOA.Serial)
					z.Notify()
				}
			case <-z.ReloadShutdown:
				watcher.remove(path.Dir(z.file))
				return
			}
		}
	}()
	return nil
}

// SOASerialIfDefined returns the SOA's serial if the zone has a SOA record in the Apex, or
// -1 otherwise.
func (z *Zone) SOASerialIfDefined() int64 {
	z.reloadMu.Lock()
	defer z.reloadMu.Unlock()
	if z.Apex.SOA != nil {
		return int64(z.Apex.SOA.Serial)
	}
	return -1
}
