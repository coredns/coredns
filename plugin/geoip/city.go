package geoip

import (
	"context"
	"strconv"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/oschwald/geoip2-golang"
)

func (g GeoIP) setCityMetadata(ctx context.Context, data *geoip2.City) {
	// Set labels for city, country and continent names.
	for _, lang := range g.langs {
		if name, ok := data.City.Names[lang]; ok {
			metadata.SetValueFunc(ctx, pluginName + "/city/names/" + lang, func() string {
				return name
			})
		}
		if name, ok := data.Country.Names[lang]; ok {
			metadata.SetValueFunc(ctx, pluginName + "/country/names/" + lang, func() string {
				return name
			})
		}
		if name, ok := data.Continent.Names[lang]; ok {
			metadata.SetValueFunc(ctx, pluginName + "/continent/names/" + lang, func() string {
				return name
			})
		}
	}

	countryCode := data.Country.IsoCode
	metadata.SetValueFunc(ctx, pluginName + "/country/code", func() string {
		return countryCode
	})
	isInEurope := strconv.FormatBool(data.Country.IsInEuropeanUnion)
	metadata.SetValueFunc(ctx, pluginName + "/country/is_in_european_union", func() string {
		return isInEurope
	})
	continentCode := data.Continent.Code
	metadata.SetValueFunc(ctx, pluginName + "/continent/code", func() string {
		return continentCode
	})

	latitude := strconv.FormatFloat(float64(data.Location.Latitude), 'f', -1, 64)
	metadata.SetValueFunc(ctx, pluginName + "/latitude", func() string {
		return latitude
	})
	longitude := strconv.FormatFloat(float64(data.Location.Longitude), 'f', -1, 64)
	metadata.SetValueFunc(ctx, pluginName + "/longitude", func() string {
		return longitude
	})
	timeZone := data.Location.TimeZone
	metadata.SetValueFunc(ctx, pluginName + "/timezone", func() string {
		return timeZone
	})
	postalCode := data.Postal.Code
	metadata.SetValueFunc(ctx, pluginName + "/postalcode", func() string {
		return postalCode
	})
}
