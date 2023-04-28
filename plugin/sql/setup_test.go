package sql

import (
	"testing"

	"github.com/coredns/caddy"
	"github.com/stretchr/testify/require"
)

func TestSql_SetupWithoutZoneDebugParam(t *testing.T) {
	setupSQL := `
	sql {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
		zone_update_duration
	}	
	`
	c := caddy.NewTestController("dns", setupSQL)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "plugin/sql: argument for 'zone_update_duration' expected", err.Error())
}

func TestSql_SetupWithZoneDefectiveParam(t *testing.T) {
	setupSql := `
	sql {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
		zone_update_duration 1
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "time: missing unit in duration \"1\"", err.Error())
}

func TestSql_SetupWithZoneCorrectParam(t *testing.T) {
	setupSql := `
	sql {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
		zone_update_duration 1s
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.Nil(t, err)
}

func TestSql_SetupWithoutDebugParam(t *testing.T) {
	setupSql := `
	sql {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
		debug
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "plugin/sql: argument for 'debug' expected", err.Error())
}

func TestSql_SetupWithDebugParam(t *testing.T) {
	setupSql := `
	sql {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
		debug true
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.Nil(t, err)
}

func TestSql_SetupWithDebugDefectParam(t *testing.T) {
	setupSql := `
	sql {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
		debug bla
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.NotNil(t, err)
}

func TestSql_SetupWithoutDSN(t *testing.T) {
	c := caddy.NewTestController("dns", `sql`)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "empty dsn detected. Please provide dsn or file parameter", err.Error())
}

func TestSql_SetupWithDSN(t *testing.T) {
	setupSql := `
	sql {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.Nil(t, err)
}

func TestSql_SetupWithDefectDSN(t *testing.T) {
	setupSql := `
	sql {
		dsn
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "plugin/sql: argument for 'dsn' expected", err.Error())
}

func TestSql_SetupWithDefectFile(t *testing.T) {
	setupSql := `
	sql {
		file
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "plugin/sql: argument for 'file' expected", err.Error())
}

func TestSql_SetupWithValidFile(t *testing.T) {
	setupSql := `
	sql {
		file tests/vault.json
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.Nil(t, err)
}

func TestSql_SetupWithValidFileAndAutomigrate(t *testing.T) {
	setupSql := `
	sql {
		file tests/vault.json
		automigrate true
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.Nil(t, err)
}

func TestSql_SetupWithValidFileAndInvalidAutomigrate(t *testing.T) {
	setupSql := `
	sql {
		file tests/vault.json
		automigrate locker
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "strconv.ParseBool: parsing \"locker\": invalid syntax", err.Error())
}

func TestSql_SetupWithValidFileAndEmptyAutomigrate(t *testing.T) {
	setupSql := `
	sql {
		file tests/vault.json
		automigrate
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "plugin/sql: argument for 'automigrate' expected", err.Error())
}

func TestSql_SetupWithValidFileUnknownParameter(t *testing.T) {
	setupSql := `
	sql {
		file tests/vault.json
		automigrate false
		ttl 360
	}	
	`
	c := caddy.NewTestController("dns", setupSql)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "plugin/sql: Testfile:5 - Error during parsing: Wrong argument count or unexpected line ending after 'ttl'", err.Error())
}

func TestSql_ConfigInvalid(t *testing.T) {
	cfg := Config{dsn: "bla", dsnFile: "blub"}
	err := cfg.Validate()
	require.NotNil(t, err)
	require.Equal(t, "only one configuration paramater possible: file or dsn; not both of them", err.Error())
}

func TestSql_ConfigFromFile_SuccessfulRead(t *testing.T) {
	cfg := Config{dsnFile: "tests/vault.json"}
	dsn, err := cfg.GetDsn()
	require.Nil(t, err)
	require.Equal(t, "sqlite3://file:ent?mode=memory&cache=shared&_fk=1", dsn)
}

func TestSql_ConfigFromFile_ErrorReadingFile(t *testing.T) {
	cfg := Config{dsnFile: "test/vault.json"}
	_, err := cfg.GetDsn()
	require.NotNil(t, err)
	require.Equal(t, "file dsn error: open test/vault.json: no such file or directory", err.Error())
}

func TestSql_ConfigFromFile_ErrorUnmarshalDefectFile(t *testing.T) {
	cfg := Config{dsnFile: "tests/defect.json"}
	_, err := cfg.GetDsn()
	require.NotNil(t, err)
	require.Equal(t, "unable to unmarshal json file: unexpected end of JSON input", err.Error())
}
