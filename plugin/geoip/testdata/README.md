# testdata
This directory contains mmdb database files used during the testing of this plugin.

# Create mmdb database files
If you need to change them to add a new value, or field the best is to recreate them, the code snipped used to create them initially is provided next.

```go
package main

import (
	"log"
	"net"
	"os"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/inserter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

const cidr = "81.2.69.142/32"

// Create new mmdb database fixtures in this directory.
func main() {
	createCityDB("GeoLite2-City.mmdb", "DBIP-City-Lite")
	// Create unknown database type.
	createCityDB("GeoLite2-UnknownDbType.mmdb", "UnknownDbType")
	// Create ASN database.
	createASNDB("GeoLite2-ASN.mmdb", "GeoLite2-ASN")
}

func createCityDB(dbName, dbType string) {
	// Load a database writer.
	writer, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: dbType})
	if err != nil {
		log.Fatal(err)
	}

	// Define and insert the new data.
	_, ip, err := net.ParseCIDR(cidr)
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

func createASNDB(dbName, dbType string) {
	// Load a database writer.
	// IncludeReservedNetworks allows inserting private IP ranges like 10.0.0.0/8.
	writer, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType:            dbType,
		IncludeReservedNetworks: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Define and insert the new data.
	_, ip, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Fatal(err)
	}

	record := mmdbtype.Map{
		"autonomous_system_number":       mmdbtype.Uint64(12345),
		"autonomous_system_organization": mmdbtype.String("Test ASN Organization"),
	}

	if err := writer.InsertFunc(ip, inserter.TopLevelMergeWith(record)); err != nil {
		log.Fatal(err)
	}

	// Add "Not routed" entry for private IP range (ASN=0).
	// This tests edge cases from iptoasn.com data where some ranges have no ASN.
	_, notRoutedIP, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		log.Fatal(err)
	}
	notRoutedRecord := mmdbtype.Map{
		"autonomous_system_number":       mmdbtype.Uint64(0),
		"autonomous_system_organization": mmdbtype.String("Not routed"),
	}
	if err := writer.InsertFunc(notRoutedIP, inserter.TopLevelMergeWith(notRoutedRecord)); err != nil {
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
