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
The supported databases use city schema such as `City` and `Enterprise` . Other databases types with different schemas are not supported yet. 

You can download a free and public City database [here](https://www.maxmind.com/en/geoip2-city).

## Syntax
* **DBFILE** the mmdb database file path.
```txt
geoip [DBFILE]
```

You can specifiy a list of languages for `names` fields.
```txt
geoip [DBFILE] {
    languages [CODE]...
}
```

* `languages` configures a list of languages for which you want to create a label, by default it's set to English: `en`, note that except for English many languages may not be available in the database, language must be available in your geoip2 database, a list of the codes supported can be found in [Locale Codes](#LocaleCodes).

## Examples
The following configuration configures the `City` database adding names in English, French, Spanish and Simplified Chinese.
```txt
. {
    geoip /opt/geoip2/db/GeoLite2-City.mmdb {
        languages en fr es zh-CN
    }
    metadata # Note that metadata plugin must be enabled as well.
}
```

## Metadatada Labels
A limited set of fields will be exported as labels, all values are stored using strings **regardless of their underlying value type**, and therefore you may have to convert it back to its original type, note that numeric values are always represented in base 10.

**LANG** Language location ISO??? code, if database does not have a name for the selected language the label
will return a nil function, you should only configure languages available in your geoIP database, it's also your responsability to check the label function is not nil before calling it.

| Label                                | Type      | Example          | Description
| :----------------------------------- | :-------- | :--------------  | :------------------
| `geoip/city/names/LANG`              | `string`  | `Cambridge`      | Then city name in LANG language, see [Locale Codes](#LocaleCodes).
| `geoip/country/code`                 | `string`  | `GB`             | Country [ISO 3166-1](https://en.wikipedia.org/wiki/ISO_3166-1) code.
| `geoip/country/names/LANG`           | `string`  | `United Kingdom` | The country name in LANG language, see [Locale Codes](#LocaleCodes).
| `geoip/country/is_in_european_union` | `bool`    | `false`          | Either `true` or `false`.
| `geoip/continent/code`               | `string`  | `EU`             | See [Continent codes](#ContinentCodes).
| `geoip/continent/names/LANG`         | `string`  | `Europe`         | The continent name in LANG language, see [Locale Codes](#LocaleCodes).
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

## Locate codes

| Value   | Language             |
| :------ | :------------------- |
| `pt-BR` | Brazilian Portuguese |
| `en`    | English              |
| `es`    | Spanish              |
| `fr`    | French               |
| `de`    | German               |
| `ja`    | Japanese             |
| `ru`    | Russian              |
| `zh-CN` | Simplified Chinese   |
