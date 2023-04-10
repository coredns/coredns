package atlas

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/coredns/coredns/plugin/atlas/ent"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

// OpenAtlasDB opens a new database connection
func OpenAtlasDB(conn string) (*ent.Client, error) {
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
	if len(schemer) == 0 {
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
	case "mysql", "mariadb":
		return getMySQLDSN(u)
	default:
		return t, cn, fmt.Errorf("unexpected scheme: %v", u.Scheme)
	}
}

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
