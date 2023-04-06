package atlas_test

import (
	"testing"

	"github.com/coredns/coredns/plugin/atlas"
	"github.com/stretchr/testify/require"
)

func TestAtlas_GetDSN_PostgresqlWithPort(t *testing.T) {
	tp, conn, err := atlas.GetDSN("postgres://postgres:secret@localhost:5432/corednsdb?sslmode=verify-full")
	require.Nil(t, err)
	require.Equal(t, "user=postgres password=secret host=localhost port=5432 dbname=corednsdb sslmode=verify-full", conn)
	require.Equal(t, tp, "postgres")
}

func TestAtlas_GetDSN_PostgresqlWithoutPort(t *testing.T) {
	tp, conn, err := atlas.GetDSN("postgres://postgres:secret@localhost/corednsdb?sslmode=verify-full")
	require.Nil(t, err)
	require.Equal(t, "user=postgres password=secret host=localhost port=5432 dbname=corednsdb sslmode=verify-full", conn)
	require.Equal(t, tp, "postgres")
}

func TestAtlas_GetDSN_PostgresqlWithoutUserInfo(t *testing.T) {
	_, _, err := atlas.GetDSN("postgres://localhost/corednsdb?sslmode=verify-full")
	require.NotNil(t, err)
	require.Equal(t, "user expected", err.Error())
}

func TestAtlas_GetDSN_PostgresqlWitUser(t *testing.T) {
	tp, conn, err := atlas.GetDSN("postgres://user@localhost/corednsdb?sslmode=verify-full")
	require.Nil(t, err)
	require.Equal(t, "user=user host=localhost port=5432 dbname=corednsdb sslmode=verify-full", conn)
	require.Equal(t, tp, "postgres")
}

func TestAtlas_GetDSN_MySQLWithPort(t *testing.T) {
	tp, conn, err := atlas.GetDSN("mysql://root:secret@localhost:3306/db?parseTime=True")
	require.Nil(t, err)
	require.Equal(t, tp, "mysql")
	require.Equal(t, "root:secret@tcp(localhost:3306)/db?parseTime=True", conn)
}

func TestAtlas_GetDSN_MySQLWithoutPort(t *testing.T) {
	tp, conn, err := atlas.GetDSN("mysql://root:secret@localhost/db?parseTime=True")
	require.Nil(t, err)
	require.Equal(t, tp, "mysql")
	require.Equal(t, "root:secret@tcp(localhost:3306)/db?parseTime=True", conn)
}

func TestAtlas_GetDSN_MySQLWithoutPassword(t *testing.T) {
	tp, conn, err := atlas.GetDSN("mysql://root@localhost/db?parseTime=True")
	require.Nil(t, err)
	require.Equal(t, tp, "mysql")
	require.Equal(t, "root@tcp(localhost:3306)/db?parseTime=True", conn)
}

func TestAtlas_GetDSN_MySQLWithoutParams(t *testing.T) {
	tp, conn, err := atlas.GetDSN("mysql://root@localhost/db")
	require.Nil(t, err)
	require.Equal(t, tp, "mysql")
	require.Equal(t, "root@tcp(localhost:3306)/db", conn)
}

func TestAtlas_GetDSN_SQLite3(t *testing.T) {
	tp, conn, err := atlas.GetDSN("sqlite3://file:ent?mode=memory&cache=shared&_fk=1")
	require.Nil(t, err)
	require.Equal(t, "file:ent?mode=memory&cache=shared&_fk=1", conn)
	require.Equal(t, tp, "sqlite3")
}
