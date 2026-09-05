package dynupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/transfer"

	"github.com/miekg/dns"
)

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	d, err := parse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	cfg := dnsserver.GetConfig(c)
	cfg.AllowOpcode(dns.OpcodeUpdate)
	cfg.AddPlugin(func(next plugin.Handler) plugin.Handler {
		d.mu.Lock()
		d.Next = next
		if d.view != nil {
			d.view.Next = next
		}
		d.mu.Unlock()
		return d
	})

	c.OnStartup(func() error {
		if h := dnsserver.GetConfig(c).Handler("transfer"); h != nil {
			if t, ok := h.(*transfer.Transfer); ok {
				d.mu.Lock()
				d.Xfer = t
				d.mu.Unlock()
			}
		}
		return nil
	})
	c.OnShutdown(d.close)

	return nil
}

func parse(c *caddy.Controller) (*DynUpdate, error) {
	if !c.Next() {
		return nil, c.ArgErr()
	}

	origin, err := parseOrigin(c.RemainingArgs(), c.ServerBlockKeys)
	if err != nil {
		return nil, err
	}

	var (
		seed        string
		seedDefined bool
		permissions []permission
		database    string
		bound       limits
	)
	seen := make(map[string]bool)
	for c.NextBlock() {
		property := c.Val()
		if property != "allow" && seen[property] {
			return nil, c.Errf("%s specified more than once", property)
		}
		seen[property] = true
		switch c.Val() {
		case "database":
			args := c.RemainingArgs()
			if len(args) != 1 || args[0] == "" {
				return nil, c.ArgErr()
			}
			database = args[0]

		case "max_records", "max_bytes", "max_update_records":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			n, err := strconv.Atoi(args[0])
			if err != nil || n <= 0 {
				return nil, c.Errf("%s requires a positive integer", property)
			}
			switch property {
			case "max_records":
				bound.records = n
			case "max_bytes":
				bound.bytes = n
			case "max_update_records":
				bound.updateRecords = n
			}
		case "file":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			if seedDefined {
				return nil, c.Err("file specified more than once")
			}
			seed, seedDefined = args[0], true

		case "allow":
			p, err := parsePermission(c.RemainingArgs(), origin)
			if err != nil {
				return nil, err
			}
			permissions = append(permissions, p)

		default:
			return nil, c.Errf("unknown property %q", c.Val())
		}
	}

	if !seedDefined {
		return nil, c.Err("file is required")
	}
	if len(permissions) == 0 {
		return nil, c.Err("at least one allow rule is required")
	}

	if !filepath.IsAbs(seed) && dnsserver.GetConfig(c).Root != "" {
		seed = filepath.Join(dnsserver.GetConfig(c).Root, seed)
	}
	d := &DynUpdate{
		Zone:        origin,
		permissions: permissions,
		limits:      bound.defaults(),
		seed:        seed,
	}
	if database != "" {
		d.database, err = databasePath(database, dnsserver.GetConfig(c).Root)
		if err != nil {
			return nil, err
		}
		s, err := d.acquireStore()
		if err != nil {
			return nil, err
		}
		s.mu.RLock()
		d.records = s.records
		copyView := *s.view
		d.view = &copyView
		s.mu.RUnlock()
		if err := releaseStore(s); err != nil {
			return nil, err
		}
		return d, nil
	}
	d.records, err = readZoneLimited(seed, origin, d.limits)
	if err != nil {
		return nil, err
	}
	d.view, err = d.build(d.records)
	if err != nil {
		return nil, fmt.Errorf("building zone %q: %w", origin, err)
	}
	return d, nil
}

func parseOrigin(args, serverBlockKeys []string) (string, error) {
	var token string
	switch {
	case len(args) == 0:
		serverZones, err := normalizeServerBlockZones(serverBlockKeys)
		if err != nil {
			return "", err
		}
		if len(serverZones) != 1 {
			return "", fmt.Errorf("zone is required when the server block does not define exactly one zone")
		}
		token = serverZones[0]
	case len(args) == 1:
		token = args[0]
	default:
		return "", fmt.Errorf("exactly one zone is allowed")
	}

	hosts := plugin.Host(token).NormalizeExact()
	if len(hosts) != 1 || hosts[0] == "" {
		return "", fmt.Errorf("invalid zone %q", token)
	}
	origin := canonicalName(hosts[0])
	if _, ok := dns.IsDomainName(origin); !ok {
		return "", fmt.Errorf("invalid zone %q", token)
	}
	return origin, nil
}

func normalizeServerBlockZones(keys []string) ([]string, error) {
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		hosts := plugin.Host(key).NormalizeExact()
		if len(hosts) != 1 || hosts[0] == "" {
			return nil, fmt.Errorf("invalid server block zone %q", key)
		}
		zone := canonicalName(hosts[0])
		if _, ok := dns.IsDomainName(zone); !ok {
			return nil, fmt.Errorf("invalid server block zone %q", key)
		}
		seen[zone] = struct{}{}
	}

	zones := make([]string, 0, len(seen))
	for zone := range seen {
		zones = append(zones, zone)
	}
	return zones, nil
}

func parsePermission(args []string, origin string) (permission, error) {
	if len(args) < 3 {
		return permission{}, fmt.Errorf("allow requires KEY NAME and at least one RR type")
	}

	if strings.TrimSpace(args[0]) == "" || args[0] == "*" || args[0] == "@" {
		return permission{}, fmt.Errorf("invalid TSIG key %q", args[0])
	}
	key := plugin.Name(args[0]).Normalize()
	if _, ok := dns.IsDomainName(key); !ok {
		return permission{}, fmt.Errorf("invalid TSIG key %q", args[0])
	}

	p := permission{key: key, name: allNames, types: make(map[uint16]struct{})}
	if args[1] != allNames {
		name := args[1]
		if name == "@" {
			name = origin
		} else {
			name = canonicalName(name)
		}
		if _, ok := dns.IsDomainName(name); !ok || !inZone(origin, name) {
			return permission{}, fmt.Errorf("allow name %q is outside zone %q", args[1], origin)
		}
		p.name = name
	}

	for _, raw := range args[2:] {
		if raw == allTypes {
			if len(args) != 3 {
				return permission{}, fmt.Errorf("wildcard RR type must be the only type in an allow rule")
			}
			p.allTypes = true
			continue
		}
		rrType, ok := parseRRType(raw)
		if !ok || !validPolicyType(rrType) {
			return permission{}, fmt.Errorf("invalid RR type %q in allow rule", raw)
		}
		p.types[rrType] = struct{}{}
	}

	return p, nil
}

func parseRRType(raw string) (uint16, bool) {
	name := strings.ToUpper(raw)
	if rrType, ok := dns.StringToType[name]; ok {
		return rrType, true
	}
	if !strings.HasPrefix(name, "TYPE") {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(name, "TYPE"), 10, 16)
	if err != nil || n == 0 || n == uint64(dns.TypeReserved) {
		return 0, false
	}
	return uint16(n), true
}

func validPolicyType(rrType uint16) bool {
	if !knownRRType(rrType) || rrType == dns.TypeNone || unsupportedRRType(rrType) {
		return false
	}
	switch rrType {
	case dns.TypeANY:
		// ANY is an UPDATE metatype for deleting all RRsets at one name,
		// and is therefore a valid explicit authorization target.
		return true
	case dns.TypeAXFR, dns.TypeIXFR, dns.TypeMAILA, dns.TypeMAILB,
		dns.TypeOPT, dns.TypeTKEY, dns.TypeTSIG:
		return false
	default:
		return true
	}
}

func readZone(path, origin string) ([]dns.RR, error) {
	return readZoneLimited(path, origin, limits{})
}

func readZoneLimited(path, origin string, bound limits) ([]dns.RR, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("opening zone file %q: %w", path, err)
	}
	defer f.Close()

	zp := dns.NewZoneParser(f, dns.Fqdn(origin), path)
	zp.SetIncludeAllowed(true)
	z := file.NewZone(origin, path)
	soaCount := 0
	bound = bound.defaults()
	count, remaining := 0, bound.bytes
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		count++
		size := dns.Len(rr)
		if count > bound.records || size > remaining {
			return nil, fmt.Errorf("zone file %q exceeds configured limits", path)
		}
		remaining -= size
		if _, ok := rr.(*dns.SOA); ok {
			soaCount++
		}
		if err := z.Insert(rr); err != nil {
			return nil, fmt.Errorf("parsing zone file %q: %w", path, err)
		}
	}
	if err := zp.Err(); err != nil {
		return nil, fmt.Errorf("parsing zone file %q: %w", path, err)
	}
	if soaCount == 0 {
		return nil, fmt.Errorf("zone %q has no SOA", origin)
	}
	if soaCount != 1 {
		return nil, fmt.Errorf("zone %q contains more than one SOA", origin)
	}

	apex, err := z.ApexIfDefined()
	if err != nil {
		return nil, fmt.Errorf("zone %q has no SOA: %w", origin, err)
	}
	records := make([]dns.RR, 0, len(apex))
	for _, rr := range apex {
		records = append(records, dns.Copy(rr))
	}
	for _, elem := range z.All() {
		for _, rr := range elem.All() {
			records = append(records, dns.Copy(rr))
		}
	}

	if err := validateRecords(records, origin); err != nil {
		return nil, fmt.Errorf("invalid zone %q: %w", origin, err)
	}
	return records, nil
}
