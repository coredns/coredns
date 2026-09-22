package kubernetes

import (
	"fmt"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/kubernetes/object"

	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func TestDefaultProcessor(t *testing.T) {
	pbuild := object.DefaultProcessor(object.ToService, nil)
	reh := cache.ResourceEventHandlerFuncs{}
	idx := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{})
	processor := pbuild(idx, reh)
	testProcessor(t, processor, idx)
}

func testProcessor(t *testing.T, processor cache.ProcessFunc, idx cache.Indexer) {
	t.Helper()
	obj := &api.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "service1", Namespace: "test1"},
		Spec: api.ServiceSpec{
			ClusterIP:  "1.2.3.4",
			ClusterIPs: []string{"1.2.3.4"},
			Ports:      []api.ServicePort{{Port: 80}},
		},
	}
	obj2 := &api.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "service2", Namespace: "test1"},
		Spec: api.ServiceSpec{
			ClusterIP:  "5.6.7.8",
			ClusterIPs: []string{"5.6.7.8"},
			Ports:      []api.ServicePort{{Port: 80}},
		},
	}

	// Add the objects
	err := processor(cache.Deltas{
		{Type: cache.Added, Object: obj.DeepCopy()},
		{Type: cache.Added, Object: obj2.DeepCopy()},
	}, false)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}
	got, exists, err := idx.Get(obj)
	if err != nil {
		t.Fatalf("get added object failed: %v", err)
	}
	if !exists {
		t.Fatal("added object not found in index")
	}
	svc, ok := got.(*object.Service)
	if !ok {
		t.Fatal("object in index was incorrect type")
	}
	if fmt.Sprintf("%v", svc.ClusterIPs) != fmt.Sprintf("%v", obj.Spec.ClusterIPs) {
		t.Fatalf("expected '%v', got '%v'", obj.Spec.ClusterIPs, svc.ClusterIPs)
	}

	// Update an object
	obj.Spec.ClusterIP = "1.2.3.5"
	err = processor(cache.Deltas{{
		Type:   cache.Updated,
		Object: obj.DeepCopy(),
	}}, false)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	got, exists, err = idx.Get(obj)
	if err != nil {
		t.Fatalf("get updated object failed: %v", err)
	}
	if !exists {
		t.Fatal("updated object not found in index")
	}
	svc, ok = got.(*object.Service)
	if !ok {
		t.Fatal("object in index was incorrect type")
	}
	if fmt.Sprintf("%v", svc.ClusterIPs) != fmt.Sprintf("%v", obj.Spec.ClusterIPs) {
		t.Fatalf("expected '%v', got '%v'", obj.Spec.ClusterIPs, svc.ClusterIPs)
	}

	// Delete an object
	err = processor(cache.Deltas{{
		Type:   cache.Deleted,
		Object: obj2.DeepCopy(),
	}}, false)
	if err != nil {
		t.Fatalf("delete test failed: %v", err)
	}
	_, exists, err = idx.Get(obj2)
	if err != nil {
		t.Fatalf("get deleted object failed: %v", err)
	}
	if exists {
		t.Fatal("deleted object found in index")
	}

	// Delete an object via tombstone
	key, _ := cache.MetaNamespaceKeyFunc(obj)
	tombstone := cache.DeletedFinalStateUnknown{Key: key, Obj: svc}
	err = processor(cache.Deltas{{
		Type:   cache.Deleted,
		Object: tombstone,
	}}, false)
	if err != nil {
		t.Fatalf("tombstone delete test failed: %v", err)
	}
	_, exists, err = idx.Get(svc)
	if err != nil {
		t.Fatalf("get tombstone deleted object failed: %v", err)
	}
	if exists {
		t.Fatal("tombstone deleted object found in index")
	}
}

func TestDefaultProcessorWithPod(t *testing.T) {
	pbuild := object.DefaultProcessor(object.ToPod, nil)
	reh := cache.ResourceEventHandlerFuncs{}
	idx := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{})
	processor := pbuild(idx, reh)
	testProcessorWithPod(t, processor, idx)
}

func testProcessorWithPod(t *testing.T, processor cache.ProcessFunc, idx cache.Indexer) {
	t.Helper()
	container := api.Container{}
	obj := &api.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "test1"},
		Spec: api.PodSpec{
			Containers: []api.Container{
				container,
			},
		},
	}
	obj2 := &api.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "test1", DeletionTimestamp: &metav1.Time{Time: time.Now()}},
		Spec: api.PodSpec{
			Containers: []api.Container{
				container,
			},
		},
	}

	// Add the pod
	err := processor(cache.Deltas{
		{
			Type:   cache.Added,
			Object: obj.DeepCopy(),
		},
	}, false)
	if err != nil {
		t.Fatalf("add pod failed: %v", err)
	}
	got, exists, err := idx.Get(obj)
	if err != nil {
		t.Fatalf("get added pod failed: %v", err)
	}
	if !exists {
		t.Fatal("added pod not found in index")
	}
	_, ok := got.(*object.Pod)
	if !ok {
		t.Fatal("pod in index was incorrect type")
	}

	// Update and delete pod having DeletionTimestamp set in one batch
	err = processor(cache.Deltas{
		{
			Type:   cache.Updated,
			Object: obj2.DeepCopy(),
		},
		{
			Type:   cache.Deleted,
			Object: obj2.DeepCopy(),
		},
	}, false)

	if err != nil {
		t.Fatalf("update or delete failed: %v", err)
	}
	_, exists, err = idx.Get(obj)
	if err != nil {
		t.Fatalf("get pod failed: %v", err)
	}
	if exists {
		t.Fatal("deleted pod still exists in the index")
	}
}
