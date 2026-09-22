//go:build !coredns_manual_registration

package dnsserver

func init() {
	if err := Register(); err != nil {
		panic(err)
	}
}
