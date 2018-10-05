package federation

import (
	"github.com/coredns/coredns/plugin/kubernetes"
	"github.com/coredns/coredns/plugin/pkg/watch"

	api "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type APIConnFederationTest struct {
	zone, region string
}

func (APIConnFederationTest) HasSynced() bool                        { return true }
func (APIConnFederationTest) Run()                                   { return }
func (APIConnFederationTest) Stop() error                            { return nil }
func (APIConnFederationTest) SvcIndexReverse(string) *api.Service  { return nil }
func (APIConnFederationTest) EpIndexReverse(string) *api.Endpoints { return nil }
func (APIConnFederationTest) Modified() int64                        { return 0 }
func (APIConnFederationTest) SetWatchChan(watch.Chan)                {}
func (APIConnFederationTest) Watch(string) error                     { return nil }
func (APIConnFederationTest) StopWatching(string)                    {}

func (APIConnFederationTest) PodIndex(string) []*api.Pod {
	a := []*api.Pod{{
		ObjectMeta: meta.ObjectMeta{
			Namespace: "podns",
		},
		Status: api.PodStatus{
			PodIP: "10.240.0.1", // Remote IP set in test.ResponseWriter
		},
	}}
	return a
}

func (APIConnFederationTest) SvcIndex(key string) *api.Service {
	svcs := map[string]*api.Service{
		"testns/svc1": {
			ObjectMeta: meta.ObjectMeta{
				Name:      "svc1",
				Namespace: "testns",
			},
			Spec: api.ServiceSpec{
				ClusterIP: "10.0.0.1",
				Ports: []api.ServicePort{{
					Name:     "http",
					Protocol: "tcp",
					Port:     80,
				}},
			},
		},
		"testns/hdls1": {
			ObjectMeta: meta.ObjectMeta{
				Name:      "hdls1",
				Namespace: "testns",
			},
			Spec: api.ServiceSpec{
				ClusterIP: api.ClusterIPNone,
			},
		},
		"testns/external": {
			ObjectMeta: meta.ObjectMeta{
				Name:      "external",
				Namespace: "testns",
			},
			Spec: api.ServiceSpec{
				ExternalName: "ext.interwebs.test",
				Ports: []api.ServicePort{{
					Name:     "http",
					Protocol: "tcp",
					Port:     80,
				}},
			},
		},
	}
	return svcs[key]
}

func (APIConnFederationTest) ServiceList() []*api.Service {
	svcs := []*api.Service{
		{
			ObjectMeta: meta.ObjectMeta{
				Name:      "svc1",
				Namespace: "testns",
			},
			Spec: api.ServiceSpec{
				ClusterIP: "10.0.0.1",
				Ports: []api.ServicePort{{
					Name:     "http",
					Protocol: "tcp",
					Port:     80,
				}},
			},
		},
		{
			ObjectMeta: meta.ObjectMeta{
				Name:      "hdls1",
				Namespace: "testns",
			},
			Spec: api.ServiceSpec{
				ClusterIP: api.ClusterIPNone,
			},
		},
		{
			ObjectMeta: meta.ObjectMeta{
				Name:      "external",
				Namespace: "testns",
			},
			Spec: api.ServiceSpec{
				ExternalName: "ext.interwebs.test",
				Ports: []api.ServicePort{{
					Name:     "http",
					Protocol: "tcp",
					Port:     80,
				}},
			},
		},
	}
	return svcs
}

func (APIConnFederationTest) EpIndex(key string) *api.Endpoints {
	eps := map[string]*api.Endpoints{
		"testns/svc1": {
			Subsets: []api.EndpointSubset{
				{
					Addresses: []api.EndpointAddress{
						{
							IP:       "172.0.0.1",
							Hostname: "ep1a",
						},
					},
					Ports: []api.EndpointPort{
						{
							Port:     80,
							Protocol: "tcp",
							Name:     "http",
						},
					},
				},
			},
			ObjectMeta: meta.ObjectMeta{
				Name:      "svc1",
				Namespace: "testns",
			},
		},
	}
	return eps[key]
}

func (APIConnFederationTest) EndpointsList() []*api.Endpoints {
	eps := []*api.Endpoints{
		{
			Subsets: []api.EndpointSubset{
				{
					Addresses: []api.EndpointAddress{
						{
							IP:       "172.0.0.1",
							Hostname: "ep1a",
						},
					},
					Ports: []api.EndpointPort{
						{
							Port:     80,
							Protocol: "tcp",
							Name:     "http",
						},
					},
				},
			},
			ObjectMeta: meta.ObjectMeta{
				Name:      "svc1",
				Namespace: "testns",
			},
		},
	}
	return eps
}

func (a APIConnFederationTest) GetNodeByName(name string) (*api.Node, error) {
	return &api.Node{
		ObjectMeta: meta.ObjectMeta{
			Name: "test.node.foo.bar",
			Labels: map[string]string{
				kubernetes.LabelRegion: a.region,
				kubernetes.LabelZone:   a.zone,
			},
		},
	}, nil
}

func (APIConnFederationTest) GetNamespaceByName(name string) (*api.Namespace, error) {
	return &api.Namespace{
		ObjectMeta: meta.ObjectMeta{
			Name: name,
		},
	}, nil
}
