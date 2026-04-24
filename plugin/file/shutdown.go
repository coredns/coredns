//go:build coredns_all || coredns_file || coredns_auto || coredns_secondary || coredns_route53 || coredns_azure || coredns_clouddns || coredns_sign

package file

// OnShutdown shuts down any running go-routines for this zone.
func (z *Zone) OnShutdown() error {
	if 0 < z.ReloadInterval {
		z.reloadShutdown <- true
	}
	return nil
}
