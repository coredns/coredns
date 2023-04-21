package atlas

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

// Define log to be a logger with the plugin name in it. This way we can just use log.Info and
// friends to log.
var log = clog.NewWithPlugin(plgName)

const (
	plgName               = "atlas"
	defaultZoneUpdateTime = 1 * time.Minute
	ErrAtlasSetupArgExp   = "argument for '%s' expected"
)

// Config holds the configuration for Atlas
type Config struct {
	automigrate    bool
	dsn            string
	dsnFile        string
	debug          bool // log sql statements
	zoneUpdateTime time.Duration
}

// Credentials type for file based dsn
type Credentials struct {
	Dsn string `json:"dsn"`
}

// Validate validates the configuration
func (c Config) Validate() error {
	if len(c.dsn) == 0 && len(c.dsnFile) == 0 {
		return fmt.Errorf("empty dsn detected. Please provide dsn or file parameter")
	}

	if len(c.dsn) > 0 && len(c.dsnFile) > 0 {
		return fmt.Errorf("only one configuration paramater possible: file or dsn; not both of them")
	}

	return nil
}

// fileIsSet returns true if the file parameter is set in the `Corefile`
func (c Config) fileIsSet() bool {
	return len(c.dsnFile) > 0
}

// GetDsn gets the dsn from file or from the `Corefile`
func (c Config) GetDsn() (string, error) {
	if c.fileIsSet() {
		return c.readDsnFile()
	}
	return c.dsn, nil
}

// readDsnFile reads the credentials from configured file
func (c Config) readDsnFile() (dsn string, err error) {
	var creds Credentials
	inputBytes, err := os.ReadFile(c.dsnFile)
	if err != nil {
		return dsn, fmt.Errorf("file dsn error: %w", err)
	}

	if err = json.Unmarshal(inputBytes, &creds); err != nil {
		return dsn, fmt.Errorf("unable to unmarshal json file: %w", err)
	}

	return creds.Dsn, nil
}

// init registers this plugin.
func init() {
	plugin.Register(plgName, setup)
}

// setup is the function that gets called when the config parser see the token "atlas". Setup is responsible
// for parsing any extra options the atlas plugin may have.
func setup(c *caddy.Controller) error {

	// set defaults
	cfg := Config{
		automigrate:    false,
		zoneUpdateTime: defaultZoneUpdateTime,
	}

	for c.Next() {
		for c.NextBlock() {
			switch c.Val() {
			case "dsn":
				args := c.RemainingArgs()
				if len(args) <= 0 {
					return plugin.Error(plgName, fmt.Errorf(ErrAtlasSetupArgExp, "dsn"))
				}
				cfg.dsn = args[0]
			case "file":
				args := c.RemainingArgs()
				if len(args) <= 0 {
					return plugin.Error(plgName, fmt.Errorf(ErrAtlasSetupArgExp, "file"))
				}
				cfg.dsnFile = args[0]
			case "automigrate":
				var err error
				args := c.RemainingArgs()
				if len(args) <= 0 {
					return plugin.Error(plgName, fmt.Errorf(ErrAtlasSetupArgExp, "automigrate"))
				}
				if cfg.automigrate, err = strconv.ParseBool(args[0]); err != nil {
					return err
				}
			case "debug":
				var err error
				args := c.RemainingArgs()
				if len(args) <= 0 {
					return plugin.Error(plgName, fmt.Errorf(ErrAtlasSetupArgExp, "debug"))
				}
				if cfg.debug, err = strconv.ParseBool(args[0]); err != nil {
					return err
				}
			case "zone_update_time":
				var err error
				var duration time.Duration
				args := c.RemainingArgs()
				if len(args) <= 0 {
					return plugin.Error(plgName, fmt.Errorf(ErrAtlasSetupArgExp, "zone_update_time"))
				}
				if duration, err = time.ParseDuration(args[0]); err != nil {
					return err
				}
				cfg.zoneUpdateTime = duration

			default:
				return plugin.Error(plgName, c.ArgErr())
			}
		}
	}

	// validate configuration
	if err := cfg.Validate(); err != nil {
		return err
	}

	dsn, err := cfg.GetDsn()
	if err != nil {
		return err
	}

	client, err := OpenAtlasDB(dsn)
	if err != nil {
		return err
	}

	defer func() {
		if err := client.Close(); err != nil {
			log.Errorf("database close error: %v", err)
		}
	}()

	if cfg.automigrate {
		ctx := context.Background()
		// Run database migration. Database user needs correct permissions
		if err := client.Schema.Create(ctx); err != nil {
			return fmt.Errorf("an error occurred while creating the table schema: %v", err)
		}
	}

	// Add the Plugin to CoreDNS, so Servers can use it in their plugin chain.
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return &Atlas{Next: next, cfg: &cfg}
	})

	return nil
}
