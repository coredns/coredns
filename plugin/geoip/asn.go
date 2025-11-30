package geoip

import (
	"context"
	"strconv"

	"github.com/coredns/coredns/plugin/metadata"

	"github.com/oschwald/geoip2-golang/v2"
)

func (g GeoIP) setASNMetadata(ctx context.Context, data *geoip2.ASN) {
	asnNumber := strconv.FormatUint(uint64(data.AutonomousSystemNumber), 10)
	metadata.SetValueFunc(ctx, pluginName+"/asn/number", func() string {
		return asnNumber
	})
	asnOrg := data.AutonomousSystemOrganization
	metadata.SetValueFunc(ctx, pluginName+"/asn/org", func() string {
		return asnOrg
	})
}
