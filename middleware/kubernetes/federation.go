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
	// TODO: Do not hard code these, pull them out of the API
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

// federation CNAMRecord returns a service record for the requested federated service
// with the target host in the federated CNAME format which the external DNS provider
// should be able to resolve
func (k *Kubernetes) federationCNAMERecord(r recordRequest) msg.Service {
	for _, node := range k.APIConn.NodeList().Items {
		// Mimic kube-dns implementation, and arbitrarily pick the first node with
		// non-empty availability-zone and region
		if node.Labels[LabelRegion] == "" || node.Labels[LabelAvailabilityZone] == "" {
			continue
		}
		for _, f := range k.Federations {
			if f.name != r.federation {
				continue
			}
			if r.endpoint == "" {
				return msg.Service{
					Key:  strings.Join([]string{msg.Path(r.zone, "coredns"), r.typeName, r.federation, r.namespace, r.service}, "/"),
					Host: strings.Join([]string{r.service, r.namespace, r.federation, r.typeName, node.Labels[LabelAvailabilityZone], node.Labels[LabelRegion], f.zone}, "."),
				}
			}
			return msg.Service{
				Key:  strings.Join([]string{msg.Path(r.zone, "coredns"), r.typeName, r.federation, r.namespace, r.service, r.endpoint}, "/"),
				Host: strings.Join([]string{r.endpoint, r.service, r.namespace, r.federation, r.typeName, node.Labels[LabelAvailabilityZone], node.Labels[LabelRegion], f.zone}, "."),
			}

		}
		break

	}
	return msg.Service{}
}
