//go:build coredns_all || coredns_kubernetes || coredns_k8s_external

package kubernetes

import clog "github.com/coredns/coredns/plugin/pkg/log"

func init() { clog.Discard() }
