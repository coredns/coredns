package iplookup

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/spf13/cast"

	"github.com/mholt/caddy"
)

func init() {
	caddy.RegisterPlugin("iplookup", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

// If we use a global instance, set it here
var globalIPL *IPLookup

type IPLookup struct {
	Next   plugin.Handler
	server *http.Server

	cacheLookup map[string]*cacheEntry
	head        *cacheEntry
	tail        *cacheEntry
	size        int
	sync.Mutex

	maxEntries  int
	maxDuration time.Duration
}

type cacheEntry struct {
	name    string
	value   string
	expires time.Time

	next *cacheEntry
	prev *cacheEntry
}

func setup(c *caddy.Controller) error {

	ipl := new(IPLookup)
	useGlobal := true

	for c.Next() {
		for c.NextBlock() {
			arg, val, err := getArgLine(c)
			if err != nil {
				return err
			}
			switch arg {
			case `listen`:
				ipl.server = &http.Server{
					Addr:    val,
					Handler: ipl,
				}
			case `entries`:
				ipl.maxEntries, err = cast.ToIntE(val)
				if err != nil {
					return fmt.Errorf("Could not parse iplookup entries value: %s", val)
				}
			case `duration`:
				ipl.maxDuration, err = cast.ToDurationE(val)
				if err != nil {
					return fmt.Errorf("Could not parse iplookup duration value: %s", val)
				}
			case `global`:
				useGlobal, err = cast.ToBoolE(val)
				if err != nil {
					return fmt.Errorf("Could not parse iplookup global value: %s", val)
				}
			default:
				return fmt.Errorf("Unknown iplookup configuration directive %s", arg)
			}
		}
	}

	// If we're using the global instance, update any configuration parameters
	if useGlobal {
		if globalIPL == nil {
			globalIPL = ipl
		}
		// Set all the values
		if ipl.maxEntries > 0 {
			globalIPL.maxEntries = ipl.maxEntries
		}
		if ipl.maxDuration > 0 {
			globalIPL.maxDuration = ipl.maxDuration
		}
		ipl = globalIPL
	}

	if ipl.server == nil {
		return fmt.Errorf("No iplookup listen directive given")
	}

	if ipl.maxDuration == 0 && ipl.maxEntries == 0 {
		return fmt.Errorf("For iplookup you must set either duration or entries")
	}

	// Initialize the cache
	if ipl.cacheLookup == nil {
		ipl.cacheLookup = make(map[string]*cacheEntry)

		// Start the cache cleaner
		if ipl.maxDuration > 0 {
			go func() {
				for {
					time.Sleep(30 * time.Second)
					ipl.Lock()
					ipl.cleanCache()
					ipl.Unlock()
				}
			}()
		}

		go func() {
			if err := ipl.server.ListenAndServe(); err != nil {
				fmt.Printf("Could not start iplookup server on %s\n", ipl.server.Addr)
			}
		}()
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		ipl.Next = next
		return ipl
	})

	return nil
}

// Name implements the Handler interface.
func (ipl *IPLookup) Name() string { return "iplookup" }

// Config parsing helper
func getArgLine(c *caddy.Controller) (name, value string, err error) {
	name = strings.ToLower(c.Val())
	if !c.NextArg() {
		err = fmt.Errorf("Missing argument to %s", name)
	}
	value = c.Val()
	if c.NextArg() {
		err = fmt.Errorf("%s only takes one argument", name)
	}
	return
}
