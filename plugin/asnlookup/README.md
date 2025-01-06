# asnlookup

## Name

*asnlookup* - Lookup ASN (Autonomous System Number) information using MaxMind GeoLite2 ASN database, then add associated ASN data to the context request.

## Description

The *asnlookup* plugin allows you to retrieve ASN data associated with an IP address. This plugin uses the [MaxMind GeoLite2 ASN database](https://dev.maxmind.com/geoip/docs/databases) to map IP addresses to their ASN and associated organization.

The retrieved data is added to the request context using the *metadata* plugin. You can then access it programmatically, for example:

```go
import (
    "github.com/coredns/coredns/plugin/metadata"
)
// ...
if getASN := metadata.ValueFunc(ctx, "asnlookup/asn"); getASN != nil {
    fmt.Printf("ASN: %s\n", getASN())
} else {
    fmt.Println("ASN metadata is not set.")
}
// ...
```

## Database

The plugin supports the [GeoLite2 ASN database](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data). Ensure you update the database regularly for accurate results.

## Syntax

```text
asnlookup [DBFILE]
```

or

```text
asnlookup [DBFILE] {
    [edns-subnet]
}
```

* **DBFILE**: The MaxMind GeoLite2 ASN database file path. The database should be updated periodically for accuracy.
* `edns-subnet`: Optional. Use the [EDNS0 subnet](https://en.wikipedia.org/wiki/EDNS_Client_Subnet) (if present) instead of the source IP of the DNS request. This is particularly useful for identifying client IPs through intermediate DNS resolvers.

  **NOTE:** Some resolvers mask parts of the client IP for privacy reasons, which may affect the accuracy of the lookup.

## Examples

The following configuration uses the GeoLite2 ASN database and considers EDNS0 subnet if present:

```txt
. {
    asnlookup /opt/geoip2/db/GeoLite2-ASN.mmdb {
        edns-subnet
    }
    metadata # The metadata plugin must be enabled.
}
```

You can use the metadata labels from *asnlookup* for more advanced configurations. For example, directing clients from specific ASNs to specific zones:

```txt
example.com {
    view specificasn {
        expr metadata('asnlookup/asn') == '58820'
    }
    asnlookup /opt/geoip2/db/GeoLite2-ASN.mmdb
    metadata
    file example.com.specificasn-db
}

example.com {
    file example.com.db
}
```

## Metadata Labels

The following metadata labels are set by the plugin. All values are stored as strings.

| Label                 | Type     | Example                           | Description                          |
|-----------------------|----------|-----------------------------------|--------------------------------------|
| `asnlookup/asn`       | `string` | `58820`                           | The Autonomous System Number (ASN). |
| `asnlookup/organization` | `string` | `Example Organization`            | The organization name associated with the ASN. |

