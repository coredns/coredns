# geoip

## Name

*geoip* - Lookup `.mmdb` ([MaxMind db file format](https://maxmind.github.io/MaxMind-DB/)) databases using the client IP, then add associated geoip data to the context request.

## Description

The *geoip* plugin allows you to enrich the data associated with Client IP addresses, e.g. geoip information like City, Country, and Network ASN. GeoIP data is commonly available in the `.mmdb` format, a database format that maps IPv4 and IPv6 addresses to data records using a binary search tree.

The data is added leveraging the *metadata* plugin, values can then be retrieved using it as well.

**Longitude example:**

```go
import (
    "strconv"
    "github.com/coredns/coredns/plugin/metadata"
)
// ...
if getLongitude := metadata.ValueFunc(ctx, "geoip/longitude"); getLongitude != nil {
    if longitude, err := strconv.ParseFloat(getLongitude(), 64); err == nil {
        // Do something useful with longitude.
    }
} else {
    // The metadata label geoip/longitude for some reason, was not set.
}
// ...
```

**City example:**

```go
import (
    "github.com/coredns/coredns/plugin/metadata"
)
// ...
if getCity := metadata.ValueFunc(ctx, "geoip/city/name"); getCity != nil {
    city := getCity()
    // Do something useful with city.
} else {
    // The metadata label geoip/city/name for some reason, was not set.
}
// ...
```

**ASN example:**

```go
import (
    "strconv"
    "github.com/coredns/coredns/plugin/metadata"
)
// ...
if getASN := metadata.ValueFunc(ctx, "geoip/asn/number"); getASN != nil {
    if asn, err := strconv.ParseUint(getASN(), 10, 32); err == nil {
        // Do something useful with asn.
    }
}
if getASNOrg := metadata.ValueFunc(ctx, "geoip/asn/org"); getASNOrg != nil {
    asnOrg := getASNOrg()
    // Do something useful with asnOrg.
}
// ...
```

## Databases

The supported databases use city schema such as `ASN`, `City`, and `Enterprise`. `.mmdb` files are generally supported, as long as their field names correctly map to the Metadata Labels below. Other database types with different schemas are not supported yet.

Free and commercial GeoIP `.mmdb` files are commonly available from vendors like [MaxMind](https://dev.maxmind.com/geoip/docs/databases), [IPinfo](https://ipinfo.io/developers/database-download), and [IPtoASN](https://iptoasn.com/) which is [Public Domain-licensed](https://opendatacommons.org/licenses/pddl/1-0/).

## Syntax

```text
geoip [DBFILE]
```

or

```text
geoip [DBFILE] {
    [edns-subnet]
}
```

* **DBFILE** the `mmdb` database file path. We recommend updating your `mmdb` database periodically for more accurate results.
* `edns-subnet`: Optional. Use [EDNS0 subnet](https://en.wikipedia.org/wiki/EDNS_Client_Subnet) (if present) for Geo IP instead of the source IP of the DNS request. This helps identifying the closest source IP address through intermediary DNS resolvers, and it also makes GeoIP testing easy: `dig +subnet=1.2.3.4 @dns-server.example.com www.geo-aware.com`.

  **NOTE:** due to security reasons, recursive DNS resolvers may mask a few bits off of the clients' IP address, which can cause inaccuracies in GeoIP resolution.

  There is no defined mask size in the standards, but there are examples: [RFC 7871's example](https://datatracker.ietf.org/doc/html/rfc7871#section-13) conceals the last 72 bits of an IPv6 source address, and NS1 Help Center [mentions](https://help.ns1.com/hc/en-us/articles/360020256573-About-the-EDNS-Client-Subnet-ECS-DNS-extension) that ECS-enabled DNS resolvers send only the first three octets (eg. /24) of the source IPv4 address.

## Examples

The following configuration configures the `City` database, and looks up geolocation based on EDNS0 subnet if present.

```txt
. {
    geoip /opt/geoip2/db/GeoLite2-City.mmdb {
      edns-subnet
    }
    metadata # Note that metadata plugin must be enabled as well.
}
```

The *view* plugin can use *geoip* metadata as selection criteria to provide GSLB functionality.
In this example, clients from the city "Exampleshire" will receive answers for `example.com` from the zone defined in 
`example.com.exampleshire-db`. All other clients will receive answers from the zone defined in `example.com.db`.
Note that the order of the two `example.com` server blocks below is important; the default viewless server block
must be last.

```txt
example.com {
    view exampleshire {
      expr metadata('geoip/city/name') == 'Exampleshire'
    }
    geoip /opt/geoip2/db/GeoLite2-City.mmdb
    metadata
    file example.com.exampleshire-db
}

example.com {
    file example.com.db
}
```

## Metadata Labels

A limited set of fields will be exported as labels, all values are stored using strings **regardless of their underlying value type**, and therefore you may have to convert it back to its original type, note that numeric values are always represented in base 10.

| Label                                | Type      | Example          | Description
| :----------------------------------- | :-------- | :--------------  | :------------------
| `geoip/city/name`                    | `string`  | `Cambridge`      | Then city name in English language.
| `geoip/country/code`                 | `string`  | `GB`             | Country [ISO 3166-1](https://en.wikipedia.org/wiki/ISO_3166-1) code.
| `geoip/country/name`                 | `string`  | `United Kingdom` | The country name in English language.
| `geoip/country/is_in_european_union` | `bool`    | `false`          | Either `true` or `false`.
| `geoip/continent/code`               | `string`  | `EU`             | See [Continent codes](#ContinentCodes).
| `geoip/continent/name`               | `string`  | `Europe`         | The continent name in English language.
| `geoip/latitude`                     | `float64` | `52.2242`        | Base 10, max available precision.
| `geoip/longitude`                    | `float64` | `0.1315`         | Base 10, max available precision.
| `geoip/timezone`                     | `string`  | `Europe/London`  | The timezone.
| `geoip/postalcode`                   | `string`  | `CB4`            | The postal code.
| `geoip/subdivisions/code`            | `string`  | `ENG,TWH`        | Comma separated [ISO 3166-2](https://en.wikipedia.org/wiki/ISO_3166-2) subdivision(region) codes, e.g. first level (province), second level (state).
| `geoip/asn/number`                   | `uint`    | `396982`         | The autonomous system number.
| `geoip/asn/org`                      | `string`  | `GOOGLE-CLOUD-PLATFORM` | The autonomous system organization.

## Continent Codes

| Value | Continent (EN) |
| :---- | :------------- |
| AF    | Africa         |
| AN    | Antarctica     |
| AS    | Asia           |
| EU    | Europe         |
| NA    | North America  |
| OC    | Oceania        |
| SA    | South America  |
