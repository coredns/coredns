package dnsserver

import (
	"flag"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"

	"github.com/mholt/caddy"
	"github.com/mholt/caddy/caddyfile"
)

const serverType = "dns"

// Any flags defined here, need to be namespaced to the serverType other
// wise they potentially clash with other server types.
func init() {
	flag.StringVar(&Port, serverType+".port", DefaultPort, "Default port")

	caddy.RegisterServerType(serverType, caddy.ServerType{
		Directives: func() []string { return Directives },
		DefaultInput: func() caddy.Input {
			return caddy.CaddyfileInput{
				Filepath:       "Corefile",
				Contents:       []byte(".:" + Port + " {\nwhoami\n}\n"),
				ServerTypeName: serverType,
			}
		},
		NewContext: newContext,
	})

}

func newContext() caddy.Context {
	return &dnsContext{keysToConfigs: make(map[string]*Config)}
}

type dnsContext struct {
	keysToConfigs map[string]*Config

	// configs is the master list of all site configs.
	configs []*Config
}

func (h *dnsContext) saveConfig(key string, cfg *Config) {
	h.configs = append(h.configs, cfg)
	h.keysToConfigs[key] = cfg
}

// KeyEnhancer : declaration for the plugins that choose to be keyEnhancers :
//  - registered like a KeyEnhancer (instead of a plugin)
//  - setup on a similar way as plugin, but earlier in the process
//  - applied on each Key, have ability to modify the Key and generate extra Keys
// NOTE: it is by choice that this KeyEnhancer cannot later apply as a plugin
type KeyEnhancer func(key ZoneAddr) []ZoneAddr

// SetupEnhancer to be implemented by each KeyEnhancer, parameters are in the dispenser (line setup function of plugins)
type SetupEnhancer func(dispenser *caddyfile.Dispenser) (KeyEnhancer, error)

//Register - to use to add a new KeyEnhancer
func Register(name string, enh SetupEnhancer) error {
	if keyEnhancerPlugins == nil {
		keyEnhancerPlugins = make(map[string]SetupEnhancer, 1)
	}
	if _, ok := keyEnhancerPlugins[name]; ok {
		return fmt.Errorf("duplicattion of a Key enhancer factory named '%v' ", name)
	}
	keyEnhancerPlugins[name] = enh
	return nil
}

//function of transformation of Keys and Tokens by applying the KeyEnhancer defined in the Token over the Keys
func expandKeys(keys []string, tokens map[string][]caddyfile.Token) ([]ZoneAddr, map[string][]caddyfile.Token, error) {

	// transform each Key in a ZoneAddr
	allZa := make([]ZoneAddr, len(keys))
	for i, k := range keys {
		za, err := normalizeZone(k)
		if err != nil {
			return nil, nil, err
		}
		allZa[i] = *za
	}

	// build the KeyEnhancers that are defined in the list of Tokens
	enhancersInvolved := []KeyEnhancer{}
	for n, tk := range tokens {
		if plugin, ok := keyEnhancerPlugins[n]; ok {
			// parse parameter of the token to []string and add to the list of enhancer to run
			disp := caddyfile.NewDispenserTokens("--try--", tk)
			enh, err := plugin(&disp)
			if err != nil {
				return nil, nil, fmt.Errorf("enhancer '%v' raise error when parsing : %v", n, err)
			}
			enhancersInvolved = append(enhancersInvolved, enh)
			// remove from the Token : this enhancer will not be involved in the plugins process
			delete(tokens, n)
		}
	}

	// Apply the enhancement to each real key and create the missing keys if option has to be applied several time
	keysToProcess := []ZoneAddr{}
	for _, k := range allZa {
		if len(enhancersInvolved) == 0 {
			keysToProcess = append(keysToProcess, k)
			continue
		}
		for _, enh := range enhancersInvolved {
			keys := enh(k)
			for _, nk := range keys {
				keysToProcess = append(keysToProcess, nk)
			}
		}
	}

	return keysToProcess, tokens, nil
}

// InspectServerBlocks make sure that everything checks out before
// executing directives and otherwise prepares the directives to
// be parsed and executed.
func (h *dnsContext) InspectServerBlocks(sourceFile string, serverBlocks []caddyfile.ServerBlock) ([]caddyfile.ServerBlock, error) {
	// Normalize and check all the zone names and check for duplicates
	// there is a dup if the same unicast is listening or a multicast is listening

	dups := newZoneAddrOverlapValidator()
	newBlocs := make([]caddyfile.ServerBlock, len(serverBlocks))
	for i, s := range serverBlocks {

		//read all keys and expands as ZoneAddr according to defined KeyEnhancer in Tokens (eg bind)
		zAddrs, tokens, err := expandKeys(s.Keys, s.Tokens)
		if err != nil {
			return nil, err
		}

		// prepare the keys for this server bloc
		keys := make([]string, len(zAddrs))
		for i, za := range zAddrs {

			// value of the key, from the expanded ZoneAddr
			currentKey := za.String()
			// prepare a config for the server block
			cfg := &Config{
				Zone:       za.Zone,
				ListenHost: za.serverAddr(),
				Port:       za.Port,
				Transport:  za.Transport,
			}

			// update specific for reverse zone
			if za.IPNet != nil {
				ones, bits := za.IPNet.Mask.Size()
				if (bits-ones)%8 != 0 { // only do this for non-octet boundaries
					cfg.FilterFunc = func(s string) bool {
						// TODO(miek): strings.ToLower! Slow and allocates new string.
						addr := dnsutil.ExtractAddressFromReverse(strings.ToLower(s))
						if addr == "" {
							return true
						}
						return za.IPNet.Contains(net.ParseIP(addr))
					}
				}
			}

			// save key and config
			keys[i] = currentKey
			h.saveConfig(currentKey, cfg)

			// Validate the overlapping of ZoneAddr
			alreadyDefined, overlapDefined, overlapKey := dups.registerAndCheck(za)
			if alreadyDefined {
				return nil, fmt.Errorf("cannot serve %s - it is already defined", za.String())
			}
			if overlapDefined {
				return nil, fmt.Errorf("cannot serve %s - zone overlap listener capacity with %v", za.String(), overlapKey)
			}

		}
		// Now save new keys and list of tokens in the serverBlock
		newBlocs[i] = caddyfile.ServerBlock{Keys: keys, Tokens: tokens}
	}

	return newBlocs, nil
}

// MakeServers uses the newly-created siteConfigs to create and return a list of server instances.
func (h *dnsContext) MakeServers() ([]caddy.Server, error) {

	// we must map (group) each config to a bind address
	groups, err := groupConfigsByListenAddr(h.configs)
	if err != nil {
		return nil, err
	}
	// then we create a server for each group
	var servers []caddy.Server
	for addr, group := range groups {
		// switch on addr
		switch Transport(addr) {
		case TransportDNS:
			s, err := NewServer(addr, group)
			if err != nil {
				return nil, err
			}
			servers = append(servers, s)

		case TransportTLS:
			s, err := NewServerTLS(addr, group)
			if err != nil {
				return nil, err
			}
			servers = append(servers, s)

		case TransportGRPC:
			s, err := NewServergRPC(addr, group)
			if err != nil {
				return nil, err
			}
			servers = append(servers, s)

		}

	}

	return servers, nil
}

// AddPlugin adds a plugin to a site's plugin stack.
func (c *Config) AddPlugin(m plugin.Plugin) {
	c.Plugin = append(c.Plugin, m)
}

// registerHandler adds a handler to a site's handler registration. Handlers
//  use this to announce that they exist to other plugin.
func (c *Config) registerHandler(h plugin.Handler) {
	if c.registry == nil {
		c.registry = make(map[string]plugin.Handler)
	}

	// Just overwrite...
	c.registry[h.Name()] = h
}

// Handler returns the plugin handler that has been added to the config under its name.
// This is useful to inspect if a certain plugin is active in this server.
// Note that this is order dependent and the order is defined in directives.go, i.e. if your plugin
// comes before the plugin you are checking; it will not be there (yet).
func (c *Config) Handler(name string) plugin.Handler {
	if c.registry == nil {
		return nil
	}
	if h, ok := c.registry[name]; ok {
		return h
	}
	return nil
}

// Handlers returns a slice of plugins that have been registered. This can be used to
// inspect and interact with registered plugins but cannot be used to remove or add plugins.
// Note that this is order dependent and the order is defined in directives.go, i.e. if your plugin
// comes before the plugin you are checking; it will not be there (yet).
func (c *Config) Handlers() []plugin.Handler {
	if c.registry == nil {
		return nil
	}
	hs := make([]plugin.Handler, 0, len(c.registry))
	for k := range c.registry {
		hs = append(hs, c.registry[k])
	}
	return hs
}

// groupSiteConfigsByListenAddr groups site configs by their listen
// (bind) address, so sites that use the same listener can be served
// on the same server instance. The return value maps the listen
// address (what you pass into net.Listen) to the list of site configs.
// This function does NOT vet the configs to ensure they are compatible.
func groupConfigsByListenAddr(configs []*Config) (map[string][]*Config, error) {
	groups := make(map[string][]*Config)

	for _, conf := range configs {
		addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(conf.ListenHost, conf.Port))
		if err != nil {
			return nil, err
		}
		addrstr := conf.Transport + "://" + addr.String()
		groups[addrstr] = append(groups[addrstr], conf)
	}

	return groups, nil
}

const (
	// DefaultPort is the default port.
	DefaultPort = "53"
	// TLSPort is the default port for DNS-over-TLS.
	TLSPort = "853"
	// GRPCPort is the default port for DNS-over-gRPC.
	GRPCPort = "443"
)

// These "soft defaults" are configurable by
// command line flags, etc.
var (
	// Port is the port we listen on by default.
	Port = DefaultPort

	// GracefulTimeout is the maximum duration of a graceful shutdown.
	GracefulTimeout time.Duration
)

var _ caddy.GracefulServer = new(Server)
var keyEnhancerPlugins map[string]SetupEnhancer
