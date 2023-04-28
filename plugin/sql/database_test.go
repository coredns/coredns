package sql

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestSQL_GetDSN_PostgresqlWithPort(t *testing.T) {
	tp, conn, err := GetDSN("postgres://postgres:secret@localhost:5432/corednsdb?sslmode=verify-full")
	require.Nil(t, err)
	require.Equal(t, "user=postgres password=secret host=localhost port=5432 dbname=corednsdb sslmode=verify-full", conn)
	require.Equal(t, tp, "postgres")
}

func TestSQL_GetDSN_PostgresqlWithoutPort(t *testing.T) {
	tp, conn, err := GetDSN("postgres://postgres:secret@localhost/corednsdb?sslmode=verify-full")
	require.Nil(t, err)
	require.Equal(t, "user=postgres password=secret host=localhost port=5432 dbname=corednsdb sslmode=verify-full", conn)
	require.Equal(t, tp, "postgres")
}

func TestSQL_GetDSN_PostgresqlWithoutUserInfo(t *testing.T) {
	_, _, err := GetDSN("postgres://localhost/corednsdb?sslmode=verify-full")
	require.NotNil(t, err)
	require.Equal(t, "user expected", err.Error())
}

func TestSQL_GetDSN_PostgresqlWitUser(t *testing.T) {
	tp, conn, err := GetDSN("postgres://user@localhost/corednsdb?sslmode=verify-full")
	require.Nil(t, err)
	require.Equal(t, "user=user host=localhost port=5432 dbname=corednsdb sslmode=verify-full", conn)
	require.Equal(t, tp, "postgres")
}

func TestSQL_GetDSN_MySQLWithPort(t *testing.T) {
	tp, conn, err := GetDSN("mysql://root:secret@localhost:3306/db?parseTime=True")
	require.Nil(t, err)
	require.Equal(t, tp, "mysql")
	require.Equal(t, "root:secret@tcp(localhost:3306)/db?parseTime=True", conn)
}

func TestSQL_GetDSN_MySQLWithoutPort(t *testing.T) {
	tp, conn, err := GetDSN("mysql://root:secret@localhost/db?parseTime=True")
	require.Nil(t, err)
	require.Equal(t, tp, "mysql")
	require.Equal(t, "root:secret@tcp(localhost:3306)/db?parseTime=True", conn)
}

func TestSQL_GetDSN_MySQLWithoutPassword(t *testing.T) {
	tp, conn, err := GetDSN("mysql://root@localhost/db?parseTime=True")
	require.Nil(t, err)
	require.Equal(t, tp, "mysql")
	require.Equal(t, "root@tcp(localhost:3306)/db?parseTime=True", conn)
}

func TestSQL_GetDSN_MySQLWithoutParams(t *testing.T) {
	tp, conn, err := GetDSN("mysql://root@localhost/db")
	require.Nil(t, err)
	require.Equal(t, tp, "mysql")
	require.Equal(t, "root@tcp(localhost:3306)/db", conn)
}

func TestSQL_GetDSN_SQLite3(t *testing.T) {
	tp, conn, err := GetDSN("sqlite3://file:ent?mode=memory&cache=shared&_fk=1")
	require.Nil(t, err)
	require.Equal(t, "file:ent?mode=memory&cache=shared&_fk=1", conn)
	require.Equal(t, tp, "sqlite3")
}

func TestSQL_GetDSN_UnexpectedSchemeTest1(t *testing.T) {
	_, _, err := GetDSN("some_strange_thing")
	require.NotNil(t, err)
	require.Equal(t, "unexpected scheme", err.Error())
}

func TestSQL_GetDSN_UnexpectedSchemeTest2(t *testing.T) {
	_, _, err := GetDSN("laladb://krank-the-film")
	require.NotNil(t, err)
	require.Equal(t, "unexpected scheme", err.Error())
}

func TestSQL_getMySQLDsn(t *testing.T) {
	_, _, err := getMySQLDSN(nil)
	require.NotNil(t, err)
	require.Equal(t, "unexpected mysql dsn", err.Error())
}

func TestSQL_getMySQLDsnWithoutUser(t *testing.T) {
	u, err := url.Parse("mysql://database:3306/db")
	require.Nil(t, err)
	_, _, err = getMySQLDSN(u)
	require.NotNil(t, err)
	require.Equal(t, "user expected", err.Error())

}

func TestSQL_getPostgresDsn(t *testing.T) {
	u, err := url.Parse("postgres://database:5432/db")
	require.Nil(t, err)
	_, _, err = getPostgresDSN(u)
	require.NotNil(t, err)
	require.Equal(t, "user expected", err.Error())
}

func TestSQL_getPostgresDsnWithNilUrl(t *testing.T) {
	_, _, err := getPostgresDSN(nil)
	require.NotNil(t, err)
	require.Equal(t, "unexpected postgres dsn", err.Error())
}

func TestSQL_OpenDB(t *testing.T) {
	_, err := OpenDB("bla://bla")
	require.NotNil(t, err)
	require.Equal(t, "unexpected scheme", err.Error())
}

func TestSQL_OpenDB_MissingZone(t *testing.T) {
	client, _ := OpenDB("sqlite3://file:ent?mode=memory&cache=shared&_fk=1")
	require.NotNil(t, client)

	_, err := client.DnsRR.Create().
		SetName("bla.com").
		SetRrtype(dns.TypeA).
		SetRrdata(`{"a":"1.2.3.4"}`).
		Save(context.Background())
	require.NotNil(t, err)
	require.Equal(t, "ent: missing required edge \"DnsRR.zone\"", err.Error())
}

func TestSQL_OpenDB_TableMigration(t *testing.T) {
	ctx := context.Background()
	client, _ := OpenDB("sqlite3://file:ent?mode=memory&cache=shared&_fk=1")
	err := client.Schema.Create(context.Background())
	require.Nil(t, err)

	zone, err := client.DnsZone.Create().
		SetName("bla.com.").
		SetNs("ns1.blubber.com.").
		SetMbox("info.blubber.com.").
		SetSerial(uint32(time.Now().Unix())).
		Save(ctx)
	if err != nil {
		require.Equal(t, "", err.Error())
	}
	require.Nil(t, err)

	dnsrr, err := client.DnsRR.Create().
		SetName("bla.com.").
		SetRrtype(dns.TypeA).
		SetRrdata(`{"a":"1.2.3.4"}`).
		SetZoneID(zone.ID).
		Save(ctx)
	if err != nil {
		fmt.Printf("%v\n", err.Error())
	}
	require.Nil(t, err)
	require.NotNil(t, dnsrr)

	require.Equal(t, "bla.com.", dnsrr.Name)
	require.Equal(t, uint32(3600), dnsrr.TTL)
	require.Equal(t, 20, len(dnsrr.ID.String()))
	require.True(t, *dnsrr.Activated)
}
