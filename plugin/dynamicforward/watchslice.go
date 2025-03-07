package dynamicforward

import (
	"context"
	_ "context"
	"fmt"
	v1 "k8s.io/api/discovery/v1"
	_ "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"log"
)

// startEndpointSliceWatcher tracks changes to the EndpointSlicesList for the specified service.
func startEndpointSliceWatcher(ctx context.Context, namespace, serviceName string, portName string, onUpdate func(newServers []string)) error {
	// Create config for Kubernetes-client
	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("[dynamicforward] failed to create in-cluster config in namespace=%s, service-name %s: %w\n", namespace, serviceName, err)
	}

	// Create client Kubernetes
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("[dynamicforward] failed to create Kubernetes client in namespace=%s, service-name %s: %w\n", namespace, serviceName, err)
	}

	// Create list/watch with filter by label
	listWatch := cache.NewFilteredListWatchFromClient(
		clientset.DiscoveryV1().RESTClient(),
		"endpointslices",
		namespace,
		func(options *metav1.ListOptions) {
			options.LabelSelector = fmt.Sprintf("kubernetes.io/service-name=%s", serviceName)
		},
	)

	//store all  slices
	esStore := cache.NewStore(cache.MetaNamespaceKeyFunc)

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			endpointSlice, ok := obj.(*v1.EndpointSlice)
			if !ok {
				log.Printf("[dynamicforward] error hadling addition EndpointSlice for service=%s: Unexpected type %T\n", serviceName, obj)
				return
			}
			esStore.Add(endpointSlice)
			handleUpdate(esStore, portName, serviceName, namespace, onUpdate)
			log.Printf("[dynamicforward] succusfuly added EndpointSlices for service=%s: %s\n", serviceName, endpointSlice.Name)
		},
		UpdateFunc: func(old, new interface{}) {
			oldEndpointSlice, ok1 := old.(*v1.EndpointSlice)
			newEndpointSlice, ok2 := new.(*v1.EndpointSlice)
			if !ok1 || !ok2 {
				log.Printf("[dynamicforward] error hadling update EndpointSlice for service=%s: Unexpected types: %T, %T\n", serviceName, old, new)
				return
			}
			esStore.Update(newEndpointSlice)
			handleUpdate(esStore, portName, serviceName, namespace, onUpdate)
			log.Printf("[dynamicforward] succusfuly updated EndpointSlices for service=%s: EndpointSlice updated: %s -> %s\n", serviceName, oldEndpointSlice.Name, newEndpointSlice.Name)
		},
		DeleteFunc: func(obj interface{}) {
			endpointSlice, ok := obj.(*v1.EndpointSlice)
			if !ok {
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					log.Printf("[dynamicforward] error delete EndpointSlice for service=%s: Unexpected type %T\n", serviceName, obj)
					return
				}
				endpointSlice, ok = tombstone.Obj.(*v1.EndpointSlice)
				if !ok {
					log.Printf("[dynamicforward] error delete EndpointSlice for service=%s: Tombstone contained object of unexpected type %T\n", serviceName, tombstone.Obj)
					return
				}
			}
			esStore.Delete(endpointSlice)
			handleUpdate(esStore, portName, serviceName, namespace, onUpdate)
			log.Printf("[dynamicforward] succusfuly seleted EndpointSlice for service=%s: EndpointSlice: %s\n", serviceName, endpointSlice.Name)
		},
	}

	// Create controller for EndpointSlice
	informerOptions := cache.InformerOptions{
		ListerWatcher:   listWatch,
		ObjectType:      &v1.EndpointSlice{},
		Handler:         handler,
		ResyncPeriod:    0,
		MinWatchTimeout: 0,
		Indexers:        nil,
		Transform:       nil,
	}

	_, controller := cache.NewInformerWithOptions(informerOptions)

	// Start informer
	go controller.Run(ctx.Done())

	// Wait while informer end sync
	if !cache.WaitForCacheSync(ctx.Done(), controller.HasSynced) {
		return fmt.Errorf("[dynamicforward] failed to sync EndpointSlices informer")
	}

	log.Printf("[dynamicforward] EndpointSlice watcher for service %s in namespace %s: is running...", serviceName, namespace)

	return nil
}

// updateServers handle update EndpointSlice and callback
func handleUpdate(store cache.Store, portName string, serviceName string, namespace string, onUpdate func(newServers []string)) {

	// Show all pslices in cache
	items := store.List()
	log.Printf("[dynamicforward] Number of EndpointSlices in cache for service %s in namespace %s: %d", serviceName, namespace, len(items))

	// Collecting a list of addresses and ports
	servers := make(map[string]struct{})
	for _, item := range items {
		endpointSlice, ok := item.(*v1.EndpointSlice)
		if !ok {
			log.Printf("[dynamicforward] Failed to cast object to EndpointSlice for service %s in namespace %s: %v", serviceName, namespace, item)
			continue
		}

		// We process all addresses and ports
		for _, endpoint := range endpointSlice.Endpoints {
			for _, address := range endpoint.Addresses {
				for _, port := range endpointSlice.Ports {
					if port.Port != nil && port.Name != nil && *port.Name == portName {
						server := fmt.Sprintf("%s:%d", address, *port.Port)
						servers[server] = struct{}{}
					}
				}
			}
		}
	}

	// convert map in slice
	serverList := make([]string, 0, len(servers)) // в противном случае вызовет ненужные ресайзы слайса
	for server := range servers {
		serverList = append(serverList, server)
	}

	// callback onUpdate
	onUpdate(serverList)
}
