package geoip

import (
	"context"

	"github.com/coredns/coredns/plugin/metadata"

	"github.com/oschwald/geoip2-golang/v2"
)

func (g GeoIP) setDomainMetadata(ctx context.Context, data *geoip2.Domain) {
	domainName := data.Domain
	metadata.SetValueFunc(ctx, pluginName+"/domain", func() string {
		return domainName
	})
}
