# geoip

## Name
*geoip* - Lookup maxmind geoip2 databases using the client IP, then add associated geoip data to the context request.

## Description
The *geoip* plugin add geo location data associated with the client IP, it allows you to configure a [geoIP2 maxmind database](https://dev.maxmind.com/geoip/docs/databases) to add the geo location data associated with the IP address.

The data is added leveraging the *metadata* plugin, values can then be retrieved using it as well, for example:

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

## Databases
The supported databases use city schema such as `City` and `Enterprise`. Other databases types with different schemas are not supported yet. 

You can download a [free and public City database](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data).

## Syntax
```txt
geoip [DBFILE]
```
* **DBFILE** the mmdb database file path.

## Examples
The following configuration configures the `City` database.
```txt
. {
    geoip /opt/geoip2/db/GeoLite2-City.mmdb
    metadata # Note that metadata plugin must be enabled as well.
}
```

## Metadatada Labels
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
