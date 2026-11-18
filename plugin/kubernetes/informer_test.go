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

// A pod that finishes while it is already being deleted (a finalizer can hold
// back the Deleted event) must drop out of the pod IP index on the terminal
// update, so a pod that reuses the IP is the only match.
func TestDefaultProcessorTerminatingPodTerminalPhase(t *testing.T) {
	for _, phase := range []api.PodPhase{api.PodSucceeded, api.PodFailed} {
		for _, ip := range []string{"10.244.0.5", "fd00::5"} {
			t.Run(string(phase)+"/"+ip, func(t *testing.T) {
				idx := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{podIPIndex: podIPIndexFunc})
				processor := object.DefaultProcessor(object.ToPod, nil)(idx, cache.ResourceEventHandlerFuncs{})
				dns := &dnsControl{podLister: idx}

				old := &api.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "old-ns", ResourceVersion: "1"},
					Status:     api.PodStatus{Phase: api.PodRunning, PodIPs: []api.PodIP{{IP: ip}}},
				}
				if err := processor(cache.Deltas{{Type: cache.Added, Object: old.DeepCopy()}}, false); err != nil {
					t.Fatalf("add pod failed: %v", err)
				}
				if pods := dns.PodIndex(ip); len(pods) != 1 || pods[0].Namespace != "old-ns" {
					t.Fatalf("expected old pod in index, got %v", pods)
				}

				done := old.DeepCopy()
				done.ResourceVersion = "2"
				done.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				done.Status.Phase = phase
				if err := processor(cache.Deltas{{Type: cache.Updated, Object: done.DeepCopy()}}, false); err != nil {
					t.Fatalf("update pod failed: %v", err)
				}
				if pods := dns.PodIndex(ip); len(pods) != 0 {
					t.Fatalf("expected no pods for %s after terminal update, got %v", ip, pods)
				}
				if _, exists, _ := idx.Get(done); !exists {
					t.Fatal("terminal pod should stay in the store until it is deleted")
				}

				replacement := &api.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "new", Namespace: "new-ns", ResourceVersion: "3"},
					Status:     api.PodStatus{Phase: api.PodRunning, PodIPs: []api.PodIP{{IP: ip}}},
				}
				if err := processor(cache.Deltas{{Type: cache.Added, Object: replacement.DeepCopy()}}, false); err != nil {
					t.Fatalf("add replacement pod failed: %v", err)
				}
				if pods := dns.PodIndex(ip); len(pods) != 1 || pods[0].Namespace != "new-ns" {
					t.Fatalf("expected only the replacement pod for %s, got %v", ip, pods)
				}

				if err := processor(cache.Deltas{{Type: cache.Deleted, Object: done.DeepCopy()}}, false); err != nil {
					t.Fatalf("delete pod failed: %v", err)
				}
				if _, exists, _ := idx.Get(done); exists {
					t.Fatal("deleted pod still in the store")
				}
			})
		}
	}
}

// A pod that is being deleted but still running keeps the current handling:
// the update is skipped and the pod stays indexed until it is deleted.
func TestDefaultProcessorTerminatingRunningPodSkipped(t *testing.T) {
	idx := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{podIPIndex: podIPIndexFunc})
	processor := object.DefaultProcessor(object.ToPod, nil)(idx, cache.ResourceEventHandlerFuncs{})
	dns := &dnsControl{podLister: idx}

	pod := &api.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns", ResourceVersion: "1"},
		Status:     api.PodStatus{Phase: api.PodRunning, PodIPs: []api.PodIP{{IP: "10.244.0.6"}}},
	}
	if err := processor(cache.Deltas{{Type: cache.Added, Object: pod.DeepCopy()}}, false); err != nil {
		t.Fatalf("add pod failed: %v", err)
	}
	terminating := pod.DeepCopy()
	terminating.ResourceVersion = "2"
	terminating.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	if err := processor(cache.Deltas{{Type: cache.Updated, Object: terminating}}, false); err != nil {
		t.Fatalf("update pod failed: %v", err)
	}
	got, exists, _ := idx.Get(pod)
	if !exists {
		t.Fatal("pod missing from the store")
	}
	if v := got.(*object.Pod).Version; v != "1" {
		t.Errorf("expected terminating running pod update to be skipped, got version %s", v)
	}
	if pods := dns.PodIndex("10.244.0.6"); len(pods) != 1 {
		t.Errorf("expected pod to stay indexed, got %v", pods)
	}
}
