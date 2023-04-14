package atlas

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAtlas_GetDSN_PostgresqlWithPort(t *testing.T) {
	tp, conn, err := GetDSN("postgres://postgres:secret@localhost:5432/corednsdb?sslmode=verify-full")
	require.Nil(t, err)
	require.Equal(t, "user=postgres password=secret host=localhost port=5432 dbname=corednsdb sslmode=verify-full", conn)
	require.Equal(t, tp, "postgres")
}

func TestAtlas_GetDSN_PostgresqlWithoutPort(t *testing.T) {
	tp, conn, err := GetDSN("postgres://postgres:secret@localhost/corednsdb?sslmode=verify-full")
	require.Nil(t, err)
	require.Equal(t, "user=postgres password=secret host=localhost port=5432 dbname=corednsdb sslmode=verify-full", conn)
	require.Equal(t, tp, "postgres")
}

func TestAtlas_GetDSN_PostgresqlWithoutUserInfo(t *testing.T) {
	_, _, err := GetDSN("postgres://localhost/corednsdb?sslmode=verify-full")
	require.NotNil(t, err)
	require.Equal(t, "atlas: user expected", err.Error())
}

func TestAtlas_GetDSN_PostgresqlWitUser(t *testing.T) {
	tp, conn, err := GetDSN("postgres://user@localhost/corednsdb?sslmode=verify-full")
	require.Nil(t, err)
	require.Equal(t, "user=user host=localhost port=5432 dbname=corednsdb sslmode=verify-full", conn)
	require.Equal(t, tp, "postgres")
}

func TestAtlas_GetDSN_MySQLWithPort(t *testing.T) {
	tp, conn, err := GetDSN("mysql://root:secret@localhost:3306/db?parseTime=True")
	require.Nil(t, err)
	require.Equal(t, tp, "mysql")
	require.Equal(t, "root:secret@tcp(localhost:3306)/db?parseTime=True", conn)
}

func TestAtlas_GetDSN_MySQLWithoutPort(t *testing.T) {
	tp, conn, err := GetDSN("mysql://root:secret@localhost/db?parseTime=True")
	require.Nil(t, err)
	require.Equal(t, tp, "mysql")
	require.Equal(t, "root:secret@tcp(localhost:3306)/db?parseTime=True", conn)
}

func TestAtlas_GetDSN_MySQLWithoutPassword(t *testing.T) {
	tp, conn, err := GetDSN("mysql://root@localhost/db?parseTime=True")
	require.Nil(t, err)
	require.Equal(t, tp, "mysql")
	require.Equal(t, "root@tcp(localhost:3306)/db?parseTime=True", conn)
}

func TestAtlas_GetDSN_MySQLWithoutParams(t *testing.T) {
	tp, conn, err := GetDSN("mysql://root@localhost/db")
	require.Nil(t, err)
	require.Equal(t, tp, "mysql")
	require.Equal(t, "root@tcp(localhost:3306)/db", conn)
}

func TestAtlas_GetDSN_SQLite3(t *testing.T) {
	tp, conn, err := GetDSN("sqlite3://file:ent?mode=memory&cache=shared&_fk=1")
	require.Nil(t, err)
	require.Equal(t, "file:ent?mode=memory&cache=shared&_fk=1", conn)
	require.Equal(t, tp, "sqlite3")
}

func TestAtlas_GetDSN_UnexpectedSchemeTest1(t *testing.T) {
	_, _, err := GetDSN("some_strange_thing")
	require.NotNil(t, err)
	require.Equal(t, "atlas: unexpected scheme", err.Error())
}

func TestAtlas_GetDSN_UnexpectedSchemeTest2(t *testing.T) {
	_, _, err := GetDSN("laladb://krank-the-film")
	require.NotNil(t, err)
	require.Equal(t, "atlas: unexpected scheme", err.Error())
}

func TestAtlas_getMySQLDsn(t *testing.T) {
	_, _, err := getMySQLDSN(nil)
	require.NotNil(t, err)
	require.Equal(t, "atlas: unexpected mysql dsn", err.Error())
}

func TestAtlas_getMySQLDsnWithoutUser(t *testing.T) {
	u, err := url.Parse("mysql://database:3306/db")
	require.Nil(t, err)
	_, _, err = getMySQLDSN(u)
	require.NotNil(t, err)
	require.Equal(t, "atlas: user expected", err.Error())

}

func TestAtlas_getPostgresDsn(t *testing.T) {
	u, err := url.Parse("postgres://database:5432/db")
	require.Nil(t, err)
	_, _, err = getPostgresDSN(u)
	require.NotNil(t, err)
	require.Equal(t, "atlas: user expected", err.Error())
}

func TestAtlas_getPostgresDsnWithNilUrl(t *testing.T) {
	_, _, err := getPostgresDSN(nil)
	require.NotNil(t, err)
	require.Equal(t, "atlas: unexpected postgres dsn", err.Error())
}

func TestAtlas_OpenAtlasDB(t *testing.T) {
	_, err := OpenAtlasDB("bla://bla")
	require.NotNil(t, err)
	require.Equal(t, "atlas: unexpected scheme", err.Error())
}

func TestAtlas_OpenAtlasDB_TableNotFound(t *testing.T) {
	client, _ := OpenAtlasDB("sqlite3://file:ent?mode=memory&cache=shared&_fk=1")
	require.NotNil(t, client)
	_, err := client.DnsRR.Create().SetName("bla.com").Save(context.Background())
	require.NotNil(t, err)
	require.Equal(t, "no such table: dns_rrs", err.Error())
}

func TestAtlas_OpenAtlasDB_TableMigration(t *testing.T) {
	client, _ := OpenAtlasDB("sqlite3://file:ent?mode=memory&cache=shared&_fk=1")
	err := client.Schema.Create(context.Background())
	require.Nil(t, err)
	dnsrr, err := client.DnsRR.Create().SetName("bla.com").Save(context.Background())
	require.Nil(t, err)
	require.NotNil(t, dnsrr)

	require.Equal(t, "bla.com", dnsrr.Name)
	require.Equal(t, int32(3600), dnsrr.TTL)
	require.Equal(t, 20, len(dnsrr.ID.String()))
	require.True(t, dnsrr.Activated)
}
