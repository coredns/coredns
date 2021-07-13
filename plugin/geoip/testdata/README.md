# testdata
This directory contains mmdb database files used during the testing of this plugin.

# Create mmdb database files
If you need to change them to add a new value, or field the best is to recreate them, the code snipped used to create them initially is provided next.

```golang
package main

import (
	"log"
	"net"
	"os"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/inserter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

const cdir = "81.2.69.142/32"

// Create new mmdb database fixtures in this directory.
func main() {
	createCityDB("GeoLite2-City.mmdb", "DBIP-City-Lite")
	// Create unkwnon database type.
	createCityDB("GeoLite2-UnknownDbType.mmdb", "UnknownDbType")
}

func createCityDB(dbName, dbType string) {
	// Load a database writer.
	writer, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: dbType})
	if err != nil {
		log.Fatal(err)
	}

	// Define and insert the new data.
	_, ip, err := net.ParseCIDR(cdir)
	if err != nil {
		log.Fatal(err)
	}

	// TODO(snebel29): Find an alternative location in Europe Union.
	record := mmdbtype.Map{
		"city": mmdbtype.Map{
			"geoname_id": mmdbtype.Uint64(2653941),
			"names":      mmdbtype.Map{
				"en": mmdbtype.String("Cambridge"),
				"es": mmdbtype.String("Cambridge"),
			},
		},
		"continent": mmdbtype.Map{
			"code":       mmdbtype.String("EU"),
			"geoname_id": mmdbtype.Uint64(6255148),
			"names":      mmdbtype.Map{
				"en": mmdbtype.String("Europe"),
				"es": mmdbtype.String("Europa"),
			},
		},
		"country": mmdbtype.Map{
			"iso_code":             mmdbtype.String("GB"),
			"geoname_id":           mmdbtype.Uint64(2635167),
			"names":                mmdbtype.Map{
				"en": mmdbtype.String("United Kingdom"),
				"es": mmdbtype.String("Reino Unido"),
			},
			"is_in_european_union": mmdbtype.Bool(true),
		},
		"location": mmdbtype.Map{
			"accuracy_radius": mmdbtype.Uint16(200),
			"latitude":        mmdbtype.Float64(52.2242),
			"longitude":       mmdbtype.Float64(0.1315),
			"metro_code":      mmdbtype.Uint64(0),
			"time_zone":       mmdbtype.String("Europe/London"),
		},
		"postal": mmdbtype.Map{
			"code": mmdbtype.String("CB4"),
		},
		"registered_country": mmdbtype.Map{
			"iso_code":             mmdbtype.String("GB"),
			"geoname_id":           mmdbtype.Uint64(2635167),
			"names":                mmdbtype.Map{"en": mmdbtype.String("United Kingdom")},
			"is_in_european_union": mmdbtype.Bool(false),
		},
		"subdivisions": mmdbtype.Slice{
			mmdbtype.Map{
				"iso_code":   mmdbtype.String("ENG"),
				"geoname_id": mmdbtype.Uint64(6269131),
				"names":      mmdbtype.Map{"en": mmdbtype.String("England")},
			},
			mmdbtype.Map{
				"iso_code":   mmdbtype.String("CAM"),
				"geoname_id": mmdbtype.Uint64(2653940),
				"names":      mmdbtype.Map{"en": mmdbtype.String("Cambridgeshire")},
			},
		},
	}

	if err := writer.InsertFunc(ip, inserter.TopLevelMergeWith(record)); err != nil {
		log.Fatal(err)
	}

	// Write the DB to the filesystem.
	fh, err := os.Create(dbName)
	if err != nil {
		log.Fatal(err)
	}
	_, err = writer.WriteTo(fh)
	if err != nil {
		log.Fatal(err)
	}
}
```
