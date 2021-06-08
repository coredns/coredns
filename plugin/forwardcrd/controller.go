package forwardcrd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dnstap"
	"github.com/coredns/coredns/plugin/forward"
	corednsv1alpha1 "github.com/coredns/coredns/plugin/forwardcrd/apis/coredns/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const defaultResyncPeriod = 0

type forwardCRDController interface {
	Run(threads int)
	HasSynced() bool
	Stop() error
}

type forwardCRDControl struct {
	client            dynamic.Interface
	scheme            *runtime.Scheme
	forwardController cache.Controller
	forwardLister     cache.Store
	workqueue         workqueue.RateLimitingInterface
	pluginMap         *PluginInstanceMap
	instancer         pluginInstancer
	tapPlugin         *dnstap.Dnstap
	namespace         string

	// stopLock is used to enforce only a single call to Stop is active.
	// Needed because we allow stopping through an http endpoint and
	// allowing concurrent stoppers leads to stack traces.
	stopLock sync.Mutex
	shutdown bool
	stopCh   chan struct{}
}

type lifecyclePluginHandler interface {
	plugin.Handler
	OnStartup() error
	OnShutdown() error
}

type pluginInstancer func(forward.ForwardConfig) (lifecyclePluginHandler, error)

func newForwardCRDController(ctx context.Context, client dynamic.Interface, scheme *runtime.Scheme, namespace string, pluginMap *PluginInstanceMap, instancer pluginInstancer) forwardCRDController {
	controller := forwardCRDControl{
		client:    client,
		scheme:    scheme,
		stopCh:    make(chan struct{}),
		namespace: namespace,
		pluginMap: pluginMap,
		instancer: instancer,
		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ForwardCRD"),
	}

	controller.forwardLister, controller.forwardController = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				if namespace != "" {
					return controller.client.Resource(corednsv1alpha1.GroupVersion.WithResource("forwards")).Namespace(namespace).List(ctx, options)
				}
				return controller.client.Resource(corednsv1alpha1.GroupVersion.WithResource("forwards")).List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				if namespace != "" {
					return controller.client.Resource(corednsv1alpha1.GroupVersion.WithResource("forwards")).Namespace(namespace).Watch(ctx, options)
				}
				return controller.client.Resource(corednsv1alpha1.GroupVersion.WithResource("forwards")).Watch(ctx, options)
			},
		},
		&unstructured.Unstructured{},
		defaultResyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				key, err := cache.MetaNamespaceKeyFunc(obj)
				if err == nil {
					controller.workqueue.Add(key)
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				key, err := cache.MetaNamespaceKeyFunc(newObj)
				if err == nil {
					controller.workqueue.Add(key)
				}
			},
			DeleteFunc: func(obj interface{}) {
				key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
				if err == nil {
					controller.workqueue.Add(key)
				}
			},
		},
	)

	return &controller
}

// Run starts the controller. Threads is the number of workers that can process
// work on the workqueue in parallel.
func (d *forwardCRDControl) Run(threads int) {
	defer utilruntime.HandleCrash()
	defer d.workqueue.ShutDown()

	go d.forwardController.Run(d.stopCh)

	if !cache.WaitForCacheSync(d.stopCh, d.forwardController.HasSynced) {
		utilruntime.HandleError(errors.New("Timed out waiting for caches to sync"))
		return
	}

	for i := 0; i < threads; i++ {
		go wait.Until(d.runWorker, time.Second, d.stopCh)
	}

	<-d.stopCh

	// Shutdown all plugins
	for _, plugin := range d.pluginMap.List() {
		plugin.OnShutdown()
	}
}

// HasSynced returns true once the controller has completed an initial resource
// listing.
func (d *forwardCRDControl) HasSynced() bool {
	return d.forwardController.HasSynced()
}

// Stop stops the controller.
func (d *forwardCRDControl) Stop() error {
	d.stopLock.Lock()
	defer d.stopLock.Unlock()

	// Only try draining the workqueue if we haven't already.
	if !d.shutdown {
		close(d.stopCh)
		d.shutdown = true

		return nil
	}

	return fmt.Errorf("shutdown already in progress")
}

func (d *forwardCRDControl) runWorker() {
	for d.processNextItem() {
	}
}

func (d *forwardCRDControl) processNextItem() bool {
	key, quit := d.workqueue.Get()
	if quit {
		return false
	}

	defer d.workqueue.Done(key)

	err := d.sync(key.(string))
	if err != nil {
		log.Errorf("Error syncing Forward %v: %v", key, err)
		d.workqueue.AddRateLimited(key)
		return true
	}

	d.workqueue.Forget(key)

	return true
}

func (d *forwardCRDControl) sync(key string) error {
	obj, exists, err := d.forwardLister.GetByKey(key)
	if err != nil {
		return err
	}

	if !exists {
		plugin := d.pluginMap.Delete(key)
		if plugin != nil {
			plugin.OnShutdown()
		}
	} else {
		f, err := d.convertToForward(obj.(runtime.Object))
		if err != nil {
			return err
		}
		forwardConfig := forward.ForwardConfig{
			From:      f.Spec.From,
			To:        f.Spec.To,
			TapPlugin: d.tapPlugin,
		}
		plugin, err := d.instancer(forwardConfig)
		if err != nil {
			return err
		}
		err = plugin.OnStartup()
		if err != nil {
			return err
		}
		oldPlugin, updated := d.pluginMap.Upsert(key, f.Spec.From, plugin)
		if updated {
			oldPlugin.OnShutdown()
		}
	}

	return nil
}

func (d *forwardCRDControl) convertToForward(obj runtime.Object) (*corednsv1alpha1.Forward, error) {
	unstructured, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("object was not Unstructured")
	}

	switch unstructured.GetKind() {
	case "Forward":
		forward := &corednsv1alpha1.Forward{}
		err := d.scheme.Convert(unstructured, forward, nil)
		return forward, err
	default:
		return nil, fmt.Errorf("unsupported object type: %T", unstructured)
	}
}
