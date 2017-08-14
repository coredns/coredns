package kubernetes

import (
	"github.com/coredns/coredns/middleware/etcd/msg"
	"github.com/coredns/coredns/request"
)

func (k *Kubernetes) Federations(state request.Request) ([]msg.Service, error) {
	return []msg.Service{{Host: "TODO"}}, nil
}
