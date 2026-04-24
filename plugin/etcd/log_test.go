//go:build coredns_all || coredns_etcd

package etcd

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
