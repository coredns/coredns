package rlc

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/coredns/coredns/plugin/pkg/log"
	"github.com/golang/groupcache"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const CacheScheme = "http"
const CachePath = "/rlc"

func (h *RlcHandler) getPoolUrl() string {
	return fmt.Sprintf("%s://%s:%d%s", CacheScheme, h.self, h.CachePort, CachePath)
}

func (h *RlcHandler) initK8s() error {
	ctx := context.Background()
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		log.Fatalf("failed loading kube config, %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("failed building kube client, %v", err)
	}

	fsel := fields.OneTermEqualSelector("metadata.name", h.serviceName).String()
	log.Debug(fmt.Sprintf("attempted to watch services with name %s", h.serviceName))
	watcher, err := clientset.CoreV1().Endpoints(h.serviceNS).Watch(ctx, metav1.ListOptions{
		FieldSelector: fsel,
	})
	if err != nil {
		log.Fatalf("failed watching endpoints, %v", err)
	}

	go func() {
		ch := watcher.ResultChan()
		for event := range ch {
			ep, ok := event.Object.(*v1.Endpoints)
			if !ok {
				log.Debug(fmt.Sprintf("unexpected type %T", ep))
			}
			var ps []string
			for _, s := range ep.Subsets {
				for _, a := range s.Addresses {
					log.Debug(fmt.Sprintf("found peer: %s", a.TargetRef.Name))
				}
			}
			log.Debug(fmt.Sprintf("setting peers %#v", ps))
			h.pool.Set(ps...)
		}
	}()
	return nil
}

func (h *RlcHandler) initGroupCache() error {
	var err error
	if h.self, err = os.Hostname(); err != nil {
		return fmt.Errorf("could not determine hostname, %v", err)
	}

	h.pool = groupcache.NewHTTPPool(h.getPoolUrl())

	if h.UseK8s {
		err = h.initK8s()
		if err != nil {
			return err
		}
	} else {
		log.Debug(fmt.Sprintf("setting peers %#v", h.staticPeers))
		h.pool.Set(h.staticPeers...)
	}

	h.group = groupcache.NewGroup(cacheName, int64(h.Capacity), groupcache.GetterFunc(
		func(ctx groupcache.Context, key string, dest groupcache.Sink) error {
			item := h.cache.Get(key)
			if item != nil {
				entry := item.Value()
				dest.SetProto(entry)
			}
			return nil
		}))

	if h.metrics != nil {
		h.groupcacheExporter = NewGroupcacheExporter(map[string]string{}, h.group)
		h.metrics.Reg.MustRegister(h.groupcacheExporter)
		h.exporter = NewExporter(map[string]string{}, h)
		h.metrics.Reg.MustRegister(h.exporter)
	}

	address := fmt.Sprintf("0.0.0.0:%d", h.CachePort)
	h.cacheServer = &http.Server{
		Addr:    address,
		Handler: h.pool,
	}

	h.wgCacheServerDone.Add(1)
	go func() {
		defer h.wgCacheServerDone.Done() // let main know we are done cleaning up

		// always returns error. ErrServerClosed on graceful close
		if err := h.cacheServer.ListenAndServe(); err != http.ErrServerClosed {
			// unexpected error. port in use?
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	return nil
}
