package object

import (
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// NewIndexerInformer is a copy of the cache.NewIndexerInformer function, but allows custom process function
func NewIndexerInformer(lw cache.ListerWatcher, objType runtime.Object, h cache.ResourceEventHandler, indexers cache.Indexers, builder ProcessorBuilder) (cache.Indexer, cache.Controller) {
	clientState := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, indexers)

	cfg := &cache.Config{
		Queue:            cache.NewDeltaFIFO(cache.MetaNamespaceKeyFunc, clientState),
		ListerWatcher:    lw,
		ObjectType:       objType,
		FullResyncPeriod: defaultResyncPeriod,
		RetryOnError:     false,
		Process:          builder(clientState, h),
	}
	return clientState, cache.New(cfg)
}

type recordLatencyFunc func(meta.Object)

// DefaultProcessor is based on the Process function from cache.NewIndexerInformer except it does a conversion.
func DefaultProcessor(convert ToFunc, recordLatency recordLatencyFunc) ProcessorBuilder {
	return func(clientState cache.Indexer, h cache.ResourceEventHandler) cache.ProcessFunc {
		return func(obj interface{}) error {
			for _, d := range obj.(cache.Deltas) {
				switch d.Type {
				case cache.Sync, cache.Added, cache.Updated:
					obj, err := convert(d.Object)
					if err != nil {
						return err
					}
					if old, exists, err := clientState.Get(obj); err == nil && exists {
						if err := clientState.Update(obj); err != nil {
							return err
						}
						h.OnUpdate(old, obj)
					} else {
						if err := clientState.Add(obj); err != nil {
							return err
						}
						h.OnAdd(obj)
					}
					if recordLatency != nil {
						recordLatency(d.Object.(meta.Object))
					}
				case cache.Deleted:
					var obj interface{}
					obj, ok := d.Object.(cache.DeletedFinalStateUnknown)
					if !ok {
						var err error
						obj, err = convert(d.Object)
						if err != nil && err != errPodTerminating {
							return err
						}
					}

					if err := clientState.Delete(obj); err != nil {
						return err
					}
					h.OnDelete(obj)
					if !ok && recordLatency != nil {
						recordLatency(d.Object.(meta.Object))
					}
				}
			}
			return nil
		}
	}
}

const defaultResyncPeriod = 0
