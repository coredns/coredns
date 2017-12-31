package kubernetes

import (
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"golang.org/x/net/context"

	"github.com/miekg/dns"
)

func TestKubernetesXFR(t *testing.T) {
	k := New([]string{"cluster.local."})
	k.APIConn = &APIConnServeTest{}

	ctx := context.TODO()
	w := dnstest.NewRecorder(&test.ResponseWriter{})
	m := &dns.Msg{}
	m.SetAxfr(k.Zones[0])

	_, err := k.ServeDNS(ctx, w, m)
	if err != nil {
		t.Error(err)
	}
}
