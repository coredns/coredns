package proxy

import (
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/healthcheck"

	"github.com/coredns/coredns/plugin/pkg/tls"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"golang.org/x/net/context"
	"google.golang.org/grpc/grpclog"
)

func pool() []*healthcheck.UpstreamHost {
	return []*healthcheck.UpstreamHost{
		{
			Name: "localhost:10053",
		},
		{
			Name: "localhost:10054",
		},
	}
}

func TestStartupShutdown(t *testing.T) {
	grpclog.SetLogger(discard{})

	upstream := &staticUpstream{
		from: ".",
		HealthCheck: healthcheck.HealthCheck{
			Hosts:       pool(),
			FailTimeout: 10 * time.Second,
			MaxFails:    1,
		},
	}
	g := newGrpcClient(nil, upstream)
	upstream.ex = g

	p := &Proxy{}
	p.Upstreams = &[]Upstream{upstream}

	err := g.OnStartup(p)
	if err != nil {
		t.Errorf("Error starting grpc client exchanger: %s", err)
		return
	}
	if len(g.clients) != len(pool()) {
		t.Errorf("Expected %d grpc clients but found %d", len(pool()), len(g.clients))
	}

	err = g.OnShutdown(p)
	if err != nil {
		t.Errorf("Error stopping grpc client exchanger: %s", err)
		return
	}
	if len(g.clients) != 0 {
		t.Errorf("Shutdown didn't remove clients, found %d", len(g.clients))
	}
	if len(g.conns) != 0 {
		t.Errorf("Shutdown didn't remove conns, found %d", len(g.conns))
	}
}

func TestRunAQuery(t *testing.T) {
	grpclog.SetLogger(discard{})

	upstream := &staticUpstream{
		from: ".",
		HealthCheck: healthcheck.HealthCheck{
			Hosts: pool(),
		},
	}
	g := newGrpcClient(nil, upstream)
	upstream.ex = g

	p := &Proxy{}
	p.Upstreams = &[]Upstream{upstream}

	err := g.OnStartup(p)
	if err != nil {
		t.Errorf("Error starting grpc client exchanger: %s", err)
		return
	}
	// verify the client is usable, or an error is properly raised
	state := request.Request{W: &test.ResponseWriter{}, Req: new(dns.Msg)}
	g.Exchange(context.TODO(), "localhost:10053", state)

	// verify that you have proper error if the hostname is unknwn or not registered
	_, err = g.Exchange(context.TODO(), "invalid:10055", state)
	if err == nil {
		t.Errorf("Expecting a proper error when querying gRPC client with invalid hostname", err)
	}

	err = g.OnShutdown(p)
	if err != nil {
		t.Errorf("Error stopping grpc client exchanger: %s", err)
		return
	}
}

func TestRunAQueryOnSecureLinkWithInvalidCert(t *testing.T) {
	grpclog.SetLogger(discard{})

	upstreamHostname := "localhost:43001"
	upstream := &staticUpstream{
		from: ".",
		HealthCheck: healthcheck.HealthCheck{
			Hosts: []*healthcheck.UpstreamHost{
				{
					Name: upstreamHostname,
				}},
		},
	}

	filename, rmFunc, err := test.TempFile("", VALID_CERT)
	if err != nil {
		t.Errorf("Error saving file : %s", err)
		return
	}
	defer rmFunc()

	tls, err := tls.NewTLSClientConfig(filename)
	if err != nil {
		t.Errorf("Error build TLS configuration  : %s", err)
		return
	}

	g := newGrpcClient(tls, upstream)
	upstream.ex = g

	p := &Proxy{}
	p.Upstreams = &[]Upstream{upstream}

	// Althougth dial will not work, it is not expected to have an error
	err = g.OnStartup(p)
	if err != nil {
		t.Errorf("Error starting grpc client exchanger: %s", err)
		return
	}

	// verify that you have proper error if the hostname is unknwn or not registered
	state := request.Request{W: &test.ResponseWriter{}, Req: new(dns.Msg)}
	_, err = g.Exchange(context.TODO(), upstreamHostname, state)
	if err == nil {
		t.Errorf("Error in Exchange process : %s ", err)
	}

	err = g.OnShutdown(p)
	if err != nil {
		t.Errorf("Error stopping grpc client exchanger: %s", err)
		return
	}
}

// discard is a Logger that outputs nothing.
type discard struct{}

func (d discard) Fatal(args ...interface{})                 {}
func (d discard) Fatalf(format string, args ...interface{}) {}
func (d discard) Fatalln(args ...interface{})               {}
func (d discard) Print(args ...interface{})                 {}
func (d discard) Printf(format string, args ...interface{}) {}
func (d discard) Println(args ...interface{})               {}

const (
	VALID_CERT = `-----BEGIN CERTIFICATE-----
	MIIDlDCCAnygAwIBAgIJAPaRnBJUE/FVMA0GCSqGSIb3DQEBBQUAMEUxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQwHhcNMTcxMTI0MTM0OTQ3WhcNMTgxMTI0MTM0OTQ3WjBF
MQswCQYDVQQGEwJBVTETMBEGA1UECAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50
ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIB
CgKCAQEAuTDeAoWS6tdZVcp/Vh3FlagbC+9Ohi5VjRXgkpcn9JopbcF5s2jpl1v+
cRpqkrmNNKLh8qOhmgdZQdh185VNe/iZ94H42qwKZ48vvnC5hLkk3MdgUT2ewgup
vZhy/Bb1bX+buCWkQa1u8SIilECMIPZHhBP4TuBUKJWK8bBEFAeUnxB5SCkX+un4
pctRlcfg8sX/ghADnp4e//YYDqex+1wQdFqM5zWhWDZAzc5Kdkyy9r+xXNfo4s1h
fI08f6F4skz1koxG2RXOzQ7OK4YxFwT2J6V72iyzUIlRGZTbYDvair/zm1kjTF1R
B1B+XLJF9oIB4BMZbekf033ZVaQ8YwIDAQABo4GGMIGDMDMGA1UdEQQsMCqHBH8A
AAGHBDR3AQGHBDR3AQCHBDR3KmSHBDR3KGSHBDR3KmWHBDR3KtIwHQYDVR0OBBYE
FFAEccLm7D/rN3fEe1fwzH7p0spAMB8GA1UdIwQYMBaAFFAEccLm7D/rN3fEe1fw
zH7p0spAMAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQEFBQADggEBAF4zqaucNcK2
GwYfijwbbtgMqPEvbReUEXsC65riAPjksJQ9L2YxQ7K0RIugRizuD1DNQam+FSb0
cZEMEKzvMUIexbhZNFINWXY2X9yUS/oZd5pWP0WYIhn6qhmLvzl9XpxNPVzBXYWe
duMECCigU2x5tAGmFa6g/pXXOoZCBRzFXwXiuNhSyhJEEwODjLZ6vgbySuU2jso3
va4FKFDdVM16s1/RYOK5oM48XytCMB/JoYoSJHPfpt8LpVNAQEHMvPvHwuZBON/z
q8HFtDjT4pBpB8AfuzwtUZ/zJ5atwxa5+ahcqRnK2kX2RSINfyEy43FZjLlvjcGa
UIRTUJK1JKg=
-----END CERTIFICATE-----`

	INVALID_CERT = `-----BEGIN CERTIFICATE-----
MIIDoDCCAoigAwIBAgIJAPUPK7+L0jF2MA0GCSqGSIb3DQEBCwUAMDkxCzAJBgNV
BAYTAlVTMRMwEQYDVQQIDApDYWxpZm9ybmlhMRUwEwYDVQQKDAxJbmZvYmxveCBJ
bmMwHhcNMTgwMTMxMjIzNzQyWhcNMjAwMTMxMjIzNzQyWjA5MQswCQYDVQQGEwJV
UzETMBEGA1UECAwKQ2FsaWZvcm5pYTEVMBMGA1UECgwMSW5mb2Jsb3ggSW5jMIIB
IjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA1xWSRb/xCmIlb4Nn5zi+uV8T
TGCka89rkGA5z3zPPzhC3zkXOxLCowQljkSwg74tx0NVxqahF3muJn6DjryciyyX
DhC4df+4yRag1Z/R9nIab2FZgbDt60EaxU1vCbpeB/0P96Pgakft7/SCkre0ZwO+
mHV5EdMzQVcHwi56XTzMXGACKVkH3e7m+pRiTlypoIoxfIfR50d78Gszt/7IEkxf
ivZTISY8ywBST/umFL2/KVLytn1RfiM6cT06iCoreKOFTCNSIxbI5UgUYCIBS/G7
7bHDOs+NdbAfniyQSPx0ApTzcau6eDSaSTm4vzZO5MasU0nUbJG/ryFdagPtGwID
AQABo4GqMIGnMFcGA1UdEQRQME6HBH8AAAGHBDR3AQGHBDR3AQCHBDR3KmSHBDR3
KGSHBDR3KmWHBDR3KtKHBDR3KGWHBLmtuN2HBLmtuN6HBLmtuN+HBLmtuOCHBLmt
uOEwHQYDVR0OBBYEFK7DMeyjOdeLB3Us43qWeVK17joXMB8GA1UdIwQYMBaAFK7D
MeyjOdeLB3Us43qWeVK17joXMAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQELBQAD
ggEBAF7JYbex4ONh2ogtWMIKotGcmeEa2RX/tEiQ6a++ugQw8C48sDbs2wDHXfF4
MN6mHUs72UuouE+8ASYaZyGWl/MKMAxYiLIXECEbh8e9VCnh9rRg2bQly9X+5UiM
V+mXuZ4dvd1K3A4VPKOQAyX4Bseo7Y+B2tUcaUEoV/jEFaBo+WXHTzhB03KKDQ1Z
IJ2QbeE9KUxag1uJ+LqDUJ58k7LdAK0xIsRJX5J48CLu1ykWluJjnk7CE0RKMfw/
/4+F4yEvYZzFWr8VEt2PvNibtPlke3Jq8OXE7aW8PEU3sWcTRS2PHe8tDq1f+uCM
pCqWchSwieGmZEqSi9kFPF1QP6Q=
-----END CERTIFICATE-----`
)
