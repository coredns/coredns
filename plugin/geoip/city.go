package geoip

import (
	"context"
	"strconv"

	"github.com/coredns/coredns/plugin/metadata"

	"github.com/oschwald/geoip2-golang"
)

const defaultLang = "en"

func (g GeoIP) setCityMetadata(ctx context.Context, data *geoip2.City) {
	// Set labels for city, country and continent names.
	cityName := data.City.Names[defaultLang]
	metadata.SetValueFunc(ctx, pluginName+"/city/name", func() string {
		return cityName
	})
	countryName := data.Country.Names[defaultLang]
	metadata.SetValueFunc(ctx, pluginName+"/country/name", func() string {
		return countryName
	})
	continentName := data.Continent.Names[defaultLang]
	metadata.SetValueFunc(ctx, pluginName+"/continent/name", func() string {
		return continentName
	})

	countryCode := data.Country.IsoCode
	metadata.SetValueFunc(ctx, pluginName+"/country/code", func() string {
		return countryCode
	})
	isInEurope := strconv.FormatBool(data.Country.IsInEuropeanUnion)
	metadata.SetValueFunc(ctx, pluginName+"/country/is_in_european_union", func() string {
		return isInEurope
	})
	continentCode := data.Continent.Code
	metadata.SetValueFunc(ctx, pluginName+"/continent/code", func() string {
		return continentCode
	})

	latitude := strconv.FormatFloat(float64(data.Location.Latitude), 'f', -1, 64)
	metadata.SetValueFunc(ctx, pluginName+"/latitude", func() string {
		return latitude
	})
	longitude := strconv.FormatFloat(float64(data.Location.Longitude), 'f', -1, 64)
	metadata.SetValueFunc(ctx, pluginName+"/longitude", func() string {
		return longitude
	})
	timeZone := data.Location.TimeZone
	metadata.SetValueFunc(ctx, pluginName+"/timezone", func() string {
		return timeZone
	})
	postalCode := data.Postal.Code
	metadata.SetValueFunc(ctx, pluginName+"/postalcode", func() string {
		return postalCode
	})
}
