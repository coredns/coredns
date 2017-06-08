package kubernetes

import (
	"strings"

	"github.com/coredns/coredns/middleware/etcd/msg"
)

type Federation struct {
	name string
	zone string
}

const (
	LabelAvailabilityZone = "failure-domain.beta.kubernetes.io/zone"
	LabelRegion           = "failure-domain.beta.kubernetes.io/region"
)

// stripFederation removes the federation segment from the segment list, if it
// matches a configured federation name.
func (k *Kubernetes) stripFederation(segs []string) (string, []string) {

	if len(segs) < 3 {
		return "", segs
	}
	for _, f := range k.Federations {
		if f.name == segs[len(segs)-2] {
			fed := segs[len(segs)-2]
			segs[len(segs)-2] = segs[len(segs)-1]
			segs = segs[:len(segs)-1]
			return fed, segs
		}
	}
	return "", segs
}

func (k *Kubernetes) federationCNAMERecord(r recordRequest) msg.Service {
	for _, node := range k.APIConn.NodeList().Items {
		if node.Labels[LabelRegion] != "" && node.Labels[LabelAvailabilityZone] != "" {
			for _, f := range k.Federations {
				if f.name == r.federation {
					if r.endpoint == "" {
						return msg.Service{
							Key:  strings.Join([]string{msg.Path(r.zone, "coredns"), r.typeName, r.federation, r.namespace, r.service}, "/"),
							Host: strings.Join([]string{r.service, r.namespace, r.federation, r.typeName, node.Labels[LabelAvailabilityZone], node.Labels[LabelRegion], f.zone}, "."),
						}
					} else {
						return msg.Service{
							Key:  strings.Join([]string{msg.Path(r.zone, "coredns"), r.typeName, r.federation, r.namespace, r.service, r.endpoint}, "/"),
							Host: strings.Join([]string{r.endpoint, r.service, r.namespace, r.federation, r.typeName, node.Labels[LabelAvailabilityZone], node.Labels[LabelRegion], f.zone}, "."),
						}
					}
				}
				return msg.Service{}
			}

		}
	}
	return msg.Service{}
}
