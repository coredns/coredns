package dnsserver_test

import (
	"errors"
	"fmt"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	_ "github.com/coredns/coredns/plugin/bind"
	_ "github.com/coredns/coredns/plugin/whoami"

	"github.com/miekg/dns"
)

func Example_embedding() {
	oldDirectives := dnsserver.Directives
	oldCaddyQuiet := caddy.Quiet
	oldDNSQuiet := dnsserver.Quiet
	defer func() {
		dnsserver.Directives = oldDirectives
		caddy.Quiet = oldCaddyQuiet
		dnsserver.Quiet = oldDNSQuiet
	}()

	// Import only the plugins the host needs and set their execution order
	// before starting the first server.
	dnsserver.Directives = []string{"bind", "whoami"}
	caddy.Quiet = true
	dnsserver.Quiet = true

	instance, err := caddy.Start(caddy.CaddyfileInput{
		Filepath:       "Corefile",
		Contents:       []byte(".:0 {\nbind 127.0.0.1\nwhoami\n}\n"),
		ServerTypeName: "dns",
	})
	if err != nil {
		panic(err)
	}
	defer func() {
		shutdownErr := errors.Join(instance.ShutdownCallbacks()...)
		stopErr := instance.Stop()
		instance.Wait()
		if err := errors.Join(shutdownErr, stopErr); err != nil {
			panic(err)
		}
	}()

	server := instance.Servers()[0].LocalAddr().String()
	query := new(dns.Msg)
	query.SetQuestion("example.org.", dns.TypeA)
	response, err := dns.Exchange(query, server)
	if err != nil {
		panic(err)
	}

	fmt.Println(response.Authoritative, response.Rcode == dns.RcodeSuccess)
	// Output: true true
}
