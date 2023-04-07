package mysql

import (
	"fmt"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() {
	plugin.Register(pluginName, setup)
}

// setup is the function that gets called when the config parser see the token "example". Setup is responsible
// for parsing any extra options the example plugin may have. The first token this function sees is "example".
func setup(c *caddy.Controller) error {
	// Get new mysql plugin
	mysql := MakeMysqlPlugin()

	// Parse configuration
	err := mysql.parseConfig(c)
	if err != nil {
		logger.Fatalf("Parse configuration err: %s", err)
	}
	mysql.queryZoneSQL = fmt.Sprintf(mysql.queryZoneSQL, mysql.zonesTable)
	mysql.queryRecordSQL = fmt.Sprintf(mysql.queryRecordSQL, mysql.recordsTable)

	logger.Debugf("Query zone SQL: %s", mysql.queryZoneSQL)
	logger.Debugf("Query record SQL: %s", mysql.queryRecordSQL)

	// Exec options when start up
	c.OnStartup(mysql.onStartup)

	// Exec options when shut down
	c.OnShutdown(mysql.onShutdown)

	// Add the Plugin to CoreDNS, so Servers can use it in their plugin chain.
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		mysql.Next = next
		return mysql
	})

	// All OK, return a nil error.
	return nil
}
