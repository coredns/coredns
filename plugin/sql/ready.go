package sql

import "github.com/coredns/coredns/plugin/sql/ent"

// Ready implements the ready.Readiness interface, once this flips to true CoreDNS
// assumes this plugin is ready for queries; it is not checked again.
func (a SQL) Ready() bool {
	var client *ent.Client
	dsn, err := a.cfg.GetDsn()
	if err != nil {
		return false
	}

	if client, err = OpenDB(dsn); err != nil {
		return false
	}

	defer client.Close()

	return true
}
