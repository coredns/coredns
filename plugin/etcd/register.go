//go:build coredns_all || coredns_etcd

package etcd

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("etcd", setup) }
