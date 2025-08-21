module github.com/coredns/coredns

// Note this minimum version requirement. CoreDNS supports the last two
// Go versions. This follows the upstream Go project support.
go 1.24.0

require (
	github.com/Azure/azure-sdk-for-go v68.0.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.30
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.13
	github.com/DataDog/dd-trace-go/v2 v2.2.2
	github.com/apparentlymart/go-cidr v1.1.0
	github.com/aws/aws-sdk-go-v2 v1.38.0
	github.com/aws/aws-sdk-go-v2/config v1.31.0
	github.com/aws/aws-sdk-go-v2/credentials v1.18.4
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.3
	github.com/aws/aws-sdk-go-v2/service/route53 v1.56.0
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.38.0
	github.com/coredns/caddy v1.1.2-0.20241029205200-8de985351a98
	github.com/dnstap/golang-dnstap v0.4.0
	github.com/expr-lang/expr v1.17.6
	github.com/farsightsec/golang-framestream v0.3.0
	github.com/go-logr/logr v1.4.3
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645
	github.com/infobloxopen/go-trees v0.0.0-20200715205103-96a057b8dfb9
	github.com/matttproud/golang_protobuf_extensions v1.0.4
	github.com/miekg/dns v1.1.68
	github.com/opentracing/opentracing-go v1.2.0
	github.com/openzipkin-contrib/zipkin-go-opentracing v0.5.0
	github.com/openzipkin/zipkin-go v0.4.3
	github.com/oschwald/geoip2-golang v1.13.0
	github.com/prometheus/client_golang v1.23.0
	github.com/prometheus/client_model v0.6.2
	github.com/prometheus/common v0.65.0
	github.com/quic-go/quic-go v0.54.0
	go.etcd.io/etcd/api/v3 v3.6.4
	go.etcd.io/etcd/client/v3 v3.6.4
	go.uber.org/automaxprocs v1.6.0
	golang.org/x/crypto v0.41.0
	golang.org/x/sys v0.35.0
	google.golang.org/api v0.247.0
	google.golang.org/grpc v1.74.2
	google.golang.org/protobuf v1.36.7
	k8s.io/api v0.33.4
	k8s.io/apimachinery v0.33.4
	k8s.io/client-go v0.33.4
	k8s.io/klog/v2 v2.130.1
	sigs.k8s.io/mcs-api v0.2.0
)

require (
	cloud.google.com/go/auth v0.16.4 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.8.0 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.22 // indirect
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.6 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/to v0.2.0 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/DataDog/appsec-internal-go v1.13.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/origindetection v0.67.0 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.67.0 // indirect
	github.com/DataDog/datadog-agent/pkg/proto v0.67.0 // indirect
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.67.0 // indirect
	github.com/DataDog/datadog-agent/pkg/trace v0.67.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.67.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.67.0 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.67.0 // indirect
	github.com/DataDog/datadog-go/v5 v5.6.0 // indirect
	github.com/DataDog/go-libddwaf/v4 v4.3.2 // indirect
	github.com/DataDog/go-runtime-metrics-internal v0.0.4-0.20250721125240-fdf1ef85b633 // indirect
	github.com/DataDog/go-sqllexer v0.1.6 // indirect
	github.com/DataDog/go-tuf v1.1.0-0.5.2 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.27.0 // indirect
	github.com/DataDog/sketches-go v1.4.7 // indirect
	github.com/Masterminds/semver/v3 v3.3.1 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.28.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.33.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.37.0 // indirect
	github.com/aws/smithy-go v1.22.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dimchansky/utfbom v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/eapache/queue/v2 v2.0.0-20230407133247-75960ed334e4 // indirect
	github.com/ebitengine/purego v0.8.3 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/flynn/go-shlex v0.0.0-20150515145356-3f9db97f8568 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic-models v0.6.9 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.6 // indirect
	github.com/googleapis/gax-go/v2 v2.15.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.3 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20250317134145-8bc96cf8fc35 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/onsi/ginkgo/v2 v2.22.1 // indirect
	github.com/onsi/gomega v1.36.2 // indirect
	github.com/opentracing-contrib/go-observer v0.0.0-20170622124052-a52f23424492 // indirect
	github.com/oschwald/maxminddb-golang v1.13.0 // indirect
	github.com/outcaste-io/ristretto v0.2.3 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/puzpuzpuz/xsync/v3 v3.5.1 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.9.0 // indirect
	github.com/shirou/gopsutil/v4 v4.25.3 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	github.com/tinylib/msgp v1.2.5 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/collector/component v1.31.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.31.0 // indirect
	go.opentelemetry.io/collector/internal/telemetry v0.125.0 // indirect
	go.opentelemetry.io/collector/pdata v1.31.0 // indirect
	go.opentelemetry.io/collector/semconv v0.125.0 // indirect
	go.opentelemetry.io/contrib/bridges/otelzap v0.10.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0 // indirect
	go.opentelemetry.io/otel v1.36.0 // indirect
	go.opentelemetry.io/otel/log v0.11.0 // indirect
	go.opentelemetry.io/otel/metric v1.36.0 // indirect
	go.opentelemetry.io/otel/sdk v1.36.0 // indirect
	go.opentelemetry.io/otel/trace v1.36.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/mock v0.5.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/exp v0.0.0-20250408133849-7e4ce0ab07d0 // indirect
	golang.org/x/mod v0.26.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/term v0.34.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	golang.org/x/tools v0.35.0 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/kube-openapi v0.0.0-20250318190949-c8a335a9a2ff // indirect
	k8s.io/utils v0.0.0-20241210054802-24370beab758 // indirect
	sigs.k8s.io/json v0.0.0-20241010143419-9aa6b5e7a4b3 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.6.0 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)
