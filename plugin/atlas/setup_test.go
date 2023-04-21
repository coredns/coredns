package atlas

import (
	"testing"

	"github.com/coredns/caddy"
	"github.com/stretchr/testify/require"
)

func TestAtlas_SetupWithoutZoneDebugParam(t *testing.T) {
	setupAtlas := `
	atlas {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
		zone_update_duration
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "plugin/atlas: argument for 'zone_update_duration' expected", err.Error())
}

func TestAtlas_SetupWithZoneDefectiveParam(t *testing.T) {
	setupAtlas := `
	atlas {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
		zone_update_duration 1
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "time: missing unit in duration \"1\"", err.Error())
}

func TestAtlas_SetupWithZoneCorrectParam(t *testing.T) {
	setupAtlas := `
	atlas {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
		zone_update_duration 1s
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.Nil(t, err)
}

func TestAtlas_SetupWithoutDebugParam(t *testing.T) {
	setupAtlas := `
	atlas {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
		debug
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "plugin/atlas: argument for 'debug' expected", err.Error())
}

func TestAtlas_SetupWithDebugParam(t *testing.T) {
	setupAtlas := `
	atlas {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
		debug true
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.Nil(t, err)
}

func TestAtlas_SetupWithDebugDefectParam(t *testing.T) {
	setupAtlas := `
	atlas {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
		debug bla
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.NotNil(t, err)
}

func TestAtlas_SetupWithoutDSN(t *testing.T) {
	c := caddy.NewTestController("dns", `atlas`)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "empty dsn detected. Please provide dsn or file parameter", err.Error())
}

func TestAtlas_SetupWithDSN(t *testing.T) {
	setupAtlas := `
	atlas {
		dsn sqlite3://file:ent?mode=memory&cache=shared&_fk=1
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.Nil(t, err)
}

func TestAtlas_SetupWithDefectDSN(t *testing.T) {
	setupAtlas := `
	atlas {
		dsn
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "plugin/atlas: argument for 'dsn' expected", err.Error())
}

func TestAtlas_SetupWithDefectFile(t *testing.T) {
	setupAtlas := `
	atlas {
		file
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "plugin/atlas: argument for 'file' expected", err.Error())
}

func TestAtlas_SetupWithValidFile(t *testing.T) {
	setupAtlas := `
	atlas {
		file tests/vault.json
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.Nil(t, err)
}

func TestAtlas_SetupWithValidFileAndAutomigrate(t *testing.T) {
	setupAtlas := `
	atlas {
		file tests/vault.json
		automigrate true
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.Nil(t, err)
}

func TestAtlas_SetupWithValidFileAndInvalidAutomigrate(t *testing.T) {
	setupAtlas := `
	atlas {
		file tests/vault.json
		automigrate locker
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "strconv.ParseBool: parsing \"locker\": invalid syntax", err.Error())
}

func TestAtlas_SetupWithValidFileAndEmptyAutomigrate(t *testing.T) {
	setupAtlas := `
	atlas {
		file tests/vault.json
		automigrate
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "plugin/atlas: argument for 'automigrate' expected", err.Error())
}

func TestAtlas_SetupWithValidFileUnknownParameter(t *testing.T) {
	setupAtlas := `
	atlas {
		file tests/vault.json
		automigrate false
		ttl 360
	}	
	`
	c := caddy.NewTestController("dns", setupAtlas)
	err := setup(c)
	require.NotNil(t, err)
	require.Equal(t, "plugin/atlas: Testfile:5 - Error during parsing: Wrong argument count or unexpected line ending after 'ttl'", err.Error())
}

func TestAtlas_ConfigInvalid(t *testing.T) {
	cfg := Config{dsn: "bla", dsnFile: "blub"}
	err := cfg.Validate()
	require.NotNil(t, err)
	require.Equal(t, "only one configuration paramater possible: file or dsn; not both of them", err.Error())
}

func TestAtlas_ConfigFromFile_SuccessfulRead(t *testing.T) {
	cfg := Config{dsnFile: "tests/vault.json"}
	dsn, err := cfg.GetDsn()
	require.Nil(t, err)
	require.Equal(t, "sqlite3://file:ent?mode=memory&cache=shared&_fk=1", dsn)
}

func TestAtlas_ConfigFromFile_ErrorReadingFile(t *testing.T) {
	cfg := Config{dsnFile: "test/vault.json"}
	_, err := cfg.GetDsn()
	require.NotNil(t, err)
	require.Equal(t, "file dsn error: open test/vault.json: no such file or directory", err.Error())
}

func TestAtlas_ConfigFromFile_ErrorUnmarshalDefectFile(t *testing.T) {
	cfg := Config{dsnFile: "tests/defect.json"}
	_, err := cfg.GetDsn()
	require.NotNil(t, err)
	require.Equal(t, "unable to unmarshal json file: unexpected end of JSON input", err.Error())
}
