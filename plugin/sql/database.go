package sql

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/coredns/coredns/plugin/sql/ent"
	"github.com/coredns/coredns/plugin/sql/ent/dnsrr"
	"github.com/coredns/coredns/plugin/sql/ent/dnszone"
	"github.com/coredns/coredns/plugin/sql/record"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/miekg/dns"
)

var (
	errSqlClient = errors.New("sql client error")
)

// OpenDB opens a new database connection and returns the database `Client`.
// If errors are fired during establishing the connection, error contains a none nil error.
func OpenDB(conn string) (*ent.Client, error) {
	tp, co, err := GetDSN(conn)
	if err != nil {
		return nil, err
	}

	return ent.Open(tp, co)
}

// GetDSN return the database type, the connection string and error if any
func GetDSN(conn string) (t, cn string, err error) {
	// we can't parse the sqlite3 url - this will fail
	// so we have to split the conn string by '//' and
	// have to check for the sqlite3 scheme
	schemer := strings.Split(conn, "//")
	if len(schemer) < 2 {
		return t, cn, fmt.Errorf("unexpected scheme")
	}
	if schemer[0] == "sqlite3:" {
		return "sqlite3", strings.Join(schemer[1:], ""), nil
	}

	var u *url.URL
	u, err = url.Parse(conn)
	if err != nil {
		return t, cn, err
	}

	switch u.Scheme {
	case "postgres", "postgresql":
		return getPostgresDSN(u)
	case "mysql", "mariadb", "maria":
		return getMySQLDSN(u)
	default:
		return t, cn, fmt.Errorf("unexpected scheme")
	}
}

// getMysqlDSN parses the given MySQL url
func getMySQLDSN(u *url.URL) (t, c string, err error) {
	var user, password, dbname string
	passwordExists := false

	if u == nil {
		return t, c, fmt.Errorf("unexpected mysql dsn")
	}

	if u.User == nil {
		return t, c, fmt.Errorf("user expected")
	}

	user = u.User.Username()
	password, passwordExists = u.User.Password()

	if len(u.Path) > 0 {
		dbname = u.Path[1:]
	}

	params := make([]string, 0)
	for key, value := range u.Query() {
		if len(value) > 0 {
			params = append(params, fmt.Sprintf("%v=%v", key, value[0]))
		}
	}

	if passwordExists {
		c = fmt.Sprintf("%s:%s@tcp(%s)/%s", user, password, getHostWithPort(u, 3306), dbname)
	} else {
		c = fmt.Sprintf("%s@tcp(%s)/%s", user, getHostWithPort(u, 3306), dbname)
	}

	if len(params) > 0 {
		c += "?" + strings.Join(params, "&")
	}

	return "mysql", c, err
}

// getHostWithPort returns the host and port string. If the port is not set the `defaultPort` will be used.
func getHostWithPort(u *url.URL, defaultPort int) string {
	if len(u.Port()) == 0 {
		return fmt.Sprintf("%s:%d", u.Hostname(), defaultPort)
	}
	return fmt.Sprintf("%s:%s", u.Hostname(), u.Port())
}

// getPostgresDSN returns the type and connection string for postgres databases
func getPostgresDSN(u *url.URL) (t, c string, err error) {
	if u == nil {
		return t, c, fmt.Errorf("unexpected postgres dsn")
	}

	var s []string
	e := strings.NewReplacer(` `, `\ `, `'`, `\'`, `\`, `\\`)
	f := func(k, v string) {
		if len(v) > 0 {
			s = append(s, k+"="+e.Replace(v))
		}
	}

	if u.User == nil {
		return t, c, fmt.Errorf("user expected")
	}

	v := u.User.Username()
	f("user", v)

	v, _ = u.User.Password()
	f("password", v)

	if host, port, err := net.SplitHostPort(u.Host); err != nil {
		f("host", u.Host)
		f("port", "5432") // set default port
	} else {
		f("host", host)
		f("port", port)
	}

	if u.Path != "" {
		f("dbname", u.Path[1:])
	}

	q := u.Query()
	for k := range q {
		f(k, q.Get(k))
	}

	return "postgres", strings.Join(s, " "), nil
}

func (a *SQL) getSqlClient() (client *ent.Client, err error) {
	dsn, err := a.cfg.GetDsn()
	if err != nil {
		return nil, err
	}

	client, err = OpenDB(dsn)
	if err != nil {
		return nil, err
	}

	if a.cfg.debug {
		client = client.Debug()
	}
	return
}

func (a *SQL) getSOARecord(ctx context.Context, client *ent.Client, zone string) (rrs []dns.RR, err error) {
	rrs = make([]dns.RR, 0)
	if client == nil {
		return rrs, errSqlClient
	}

	soaRec, err := client.DnsZone.Query().
		Where(
			dnszone.Activated(true),
			dnszone.NameEQ(zone),
		).
		First(ctx)

	if err != nil {
		return rrs, err
	}

	rec := &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   soaRec.Name,
			Rrtype: soaRec.Rrtype,
			Class:  soaRec.Class,
			Ttl:    soaRec.TTL,
		},
		Ns:      soaRec.Ns,
		Mbox:    soaRec.Mbox,
		Serial:  soaRec.Serial,
		Refresh: soaRec.Refresh,
		Retry:   soaRec.Retry,
		Expire:  soaRec.Expire,
		Minttl:  soaRec.Minttl,
	}

	return []dns.RR{rec}, nil
}

func (a *SQL) loadZones(ctx context.Context, client *ent.Client) error {
	zones := []string{}

	if client == nil {
		return errSqlClient
	}

	records, err := client.DnsZone.
		Query().
		Select(dnszone.FieldName).
		Where(dnszone.Activated(true)).
		Order(ent.Asc(dnszone.FieldName)).
		All(ctx)
	if err != nil {
		return err
	}

	for _, zone := range records {
		zones = append(zones, zone.Name)
	}

	a.zones = zones
	a.lastZoneUpdate = time.Now()

	return nil
}

func (a *SQL) getNameServers(ctx context.Context, client *ent.Client, reqName string) (rrs []dns.RR, err error) {
	return a.getRRecords(ctx, client, reqName, dns.TypeNS)
}

func (a *SQL) getRRecords(ctx context.Context, client *ent.Client, reqName string, reqQType uint16) (rrs []dns.RR, err error) {
	rrs = make([]dns.RR, 0)
	if client == nil {
		return rrs, errSqlClient
	}

	records, err := client.DnsRR.Query().
		Select(
			dnsrr.FieldName,
			dnsrr.FieldClass,
			dnsrr.FieldRrtype,
			dnsrr.FieldRrdata,
			dnsrr.FieldTTL,
		).
		Where(
			dnsrr.NameEQ(reqName),
			dnsrr.RrtypeEQ(reqQType),
			dnsrr.ActivatedEQ(true), // we serve only activated records
		).
		Order(ent.Asc(
			dnsrr.FieldName,
			dnsrr.FieldRrtype,
		)).
		All(ctx)
	if err != nil {
		return rrs, err
	}

	for _, r := range records {
		rec, err := record.From(r)
		if err != nil {
			return rrs, err
		}
		rrs = append(rrs, rec)
	}

	return rrs, nil
}
