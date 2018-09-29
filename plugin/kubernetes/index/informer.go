package index

import (
	"errors"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// NewInformer is a copy of the cache.NewIndexInformer function, but allows Process to have a conversion function (ToFunc).
func NewInformer(lw cache.ListerWatcher, objType runtime.Object, r time.Duration, h cache.ResourceEventHandler, indexers cache.Indexers, convert ToFunc) (cache.Indexer, cache.Controller) {
	clientState := cache.NewIndexer(deletionHandlingMetaNamespaceKeyFunc, indexers)

	fifo := cache.NewDeltaFIFO(metaNamespaceKeyFunc, clientState)

	cfg := &cache.Config{
		Queue:            fifo,
		ListerWatcher:    lw,
		ObjectType:       objType,
		FullResyncPeriod: r,
		RetryOnError:     false,
		Process: func(obj interface{}) error {
			fmt.Fprintf(os.Stderr, "%T", obj)
			for _, d := range obj.(cache.Deltas) {

				obj := convert(d.Object)

				switch d.Type {
				case cache.Sync, cache.Added, cache.Updated:
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
				case cache.Deleted:
					if err := clientState.Delete(obj); err != nil {
						return err
					}
					h.OnDelete(obj)
				}
			}
			return nil
		},
	}
	return clientState, cache.New(cfg)
}

func metaNamespaceKeyFunc(obj interface{}) (string, error) {
	fmt.Fprintf(os.Stderr, "%T", obj)
	o, ok := obj.(Object)
	if !ok {
		return "", errObj
	}
	return o.UniqueKey(), nil
}

func deletionHandlingMetaNamespaceKeyFunc(obj interface{}) (string, error) {
	fmt.Fprintf(os.Stderr, "%T", obj)
	// if d, ok := obj.(DeletedFinalStateUnknown); ok {
	//	return d.Key, nil
	// }
	return metaNamespaceKeyFunc(obj)
}

var errObj = errors.New("wrong object")
