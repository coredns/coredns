package forwardcrd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/forward"
	corednsv1alpha1 "github.com/coredns/coredns/plugin/forwardcrd/apis/coredns/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic/fake"
)

func TestCreateForward(t *testing.T) {
	controller, client, testPluginInstancer, pluginInstanceMap := setupControllerTestcase(t, "")
	forward := &corednsv1alpha1.Forward{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dns-zone",
			Namespace: "default",
		},
		Spec: corednsv1alpha1.ForwardSpec{
			From: "crd.test",
			To:   []string{"127.0.0.2", "127.0.0.3"},
		},
	}

	_, err := client.Resource(corednsv1alpha1.GroupVersion.WithResource("forwards")).
		Namespace("default").
		Create(context.Background(), mustForwardToUnstructured(forward), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	err = wait.Poll(time.Second, time.Second*5, func() (bool, error) {
		return testPluginInstancer.NewWithConfigCallCount() == 1, nil
	})
	if err != nil {
		t.Fatalf("Expected plugin instance to have been called: %s", err)
	}

	handler := testPluginInstancer.NewWithConfigArgsForCall(0)
	if handler.ReceivedConfig.From != "crd.test" {
		t.Fatalf("Expected plugin to be created for zone: %s but was: %s", "crd.test", handler.ReceivedConfig.From)
	}

	if len(handler.ReceivedConfig.To) != 2 {
		t.Fatalf("Expected plugin to contain exactly 2 servers to forward to but contains: %#v", handler.ReceivedConfig.To)
	}

	if handler.ReceivedConfig.To[0] != "127.0.0.2" {
		t.Fatalf("Expected plugin to be created to forward to: %s but was: %s", "127.0.0.2", handler.ReceivedConfig.To[0])
	}

	if handler.ReceivedConfig.To[1] != "127.0.0.3" {
		t.Fatalf("Expected plugin to be created to forward to: %s but was: %s", "127.0.0.3", handler.ReceivedConfig.To[1])
	}

	pluginHandler, ok := pluginInstanceMap.Get("crd.test")
	if !ok {
		t.Fatal("Expected plugin lookup to succeed")
	}

	if pluginHandler != handler {
		t.Fatalf("Exepcted plugin lookup to match what the instancer provided: %#v but was %#v", handler, pluginHandler)
	}

	if testPluginInstancer.testPluginHandlers[0].OnStartupCallCount() != 1 {
		t.Fatalf("Expected plugin OnStartup to have been called once, but got: %d", testPluginInstancer.testPluginHandlers[0].OnStartupCallCount())
	}

	if err := controller.Stop(); err != nil {
		t.Fatalf("Expected no error: %s", err)
	}

	err = wait.Poll(time.Second, time.Second*5, func() (bool, error) {
		return testPluginInstancer.testPluginHandlers[0].OnShutdownCallCount() == 1, nil
	})
	if err != nil {
		t.Fatalf("Expected plugin OnShutdown to have been called once, but got: %d", testPluginInstancer.testPluginHandlers[0].OnShutdownCallCount())
	}
}

func TestUpdateForward(t *testing.T) {
	controller, client, testPluginInstancer, pluginInstanceMap := setupControllerTestcase(t, "")
	forward := &corednsv1alpha1.Forward{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dns-zone",
			Namespace: "default",
		},
		Spec: corednsv1alpha1.ForwardSpec{
			From: "crd.test",
			To:   []string{"127.0.0.2"},
		},
	}

	unstructuredForward, err := client.Resource(corednsv1alpha1.GroupVersion.WithResource("forwards")).
		Namespace("default").
		Create(context.Background(), mustForwardToUnstructured(forward), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	err = wait.Poll(time.Second, time.Second*5, func() (bool, error) {
		return testPluginInstancer.NewWithConfigCallCount() == 1, nil
	})
	if err != nil {
		t.Fatalf("Expected plugin instance to have been called: %s", err)
	}

	forward = mustUnstructuredToForward(unstructuredForward)
	forward.Spec.From = "other.test"
	forward.Spec.To = []string{"127.0.0.3"}

	_, err = client.Resource(corednsv1alpha1.GroupVersion.WithResource("forwards")).
		Namespace("default").
		Update(context.Background(), mustForwardToUnstructured(forward), metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	err = wait.Poll(time.Second, time.Second*5, func() (bool, error) {
		return testPluginInstancer.NewWithConfigCallCount() == 2, nil
	})
	if err != nil {
		t.Fatalf("Expected plugin instance to have been called: %s", err)
	}

	handler := testPluginInstancer.NewWithConfigArgsForCall(1)
	if handler.ReceivedConfig.From != "other.test" {
		t.Fatalf("Expected plugin to be created for zone: %s but was: %s", "other.test", handler.ReceivedConfig.From)
	}

	if len(handler.ReceivedConfig.To) != 1 {
		t.Fatalf("Expected plugin to contain exactly 1 server to forward to but contains: %#v", handler.ReceivedConfig.To)
	}

	if handler.ReceivedConfig.To[0] != "127.0.0.3" {
		t.Fatalf("Expected plugin to be created to forward to: %s but was: %s", "127.0.0.3", handler.ReceivedConfig.To[0])
	}

	pluginHandler, ok := pluginInstanceMap.Get("other.test")
	if !ok {
		t.Fatal("Expected plugin lookup to succeed")
	}

	if pluginHandler != handler {
		t.Fatalf("Exepcted plugin lookup to match what the instancer provided: %#v but was %#v", handler, pluginHandler)
	}

	_, ok = pluginInstanceMap.Get("crd.test")
	if ok {
		t.Fatal("Expected lookup for crd.test to fail")
	}

	if testPluginInstancer.testPluginHandlers[0].OnShutdownCallCount() != 1 {
		t.Fatalf("Expected plugin OnShutdown to have been called once, but got: %d", testPluginInstancer.testPluginHandlers[0].OnShutdownCallCount())
	}

	if err := controller.Stop(); err != nil {
		t.Fatalf("Expected no error: %s", err)
	}
}

func TestDeleteForward(t *testing.T) {
	controller, client, testPluginInstancer, pluginInstanceMap := setupControllerTestcase(t, "")
	forward := &corednsv1alpha1.Forward{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dns-zone",
			Namespace: "default",
		},
		Spec: corednsv1alpha1.ForwardSpec{
			From: "crd.test",
			To:   []string{"127.0.0.2"},
		},
	}

	_, err := client.Resource(corednsv1alpha1.GroupVersion.WithResource("forwards")).
		Namespace("default").
		Create(context.Background(), mustForwardToUnstructured(forward), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	err = wait.Poll(time.Second, time.Second*5, func() (bool, error) {
		return testPluginInstancer.NewWithConfigCallCount() == 1, nil
	})
	if err != nil {
		t.Fatalf("Expected plugin instance to have been called: %s", err)
	}

	err = client.Resource(corednsv1alpha1.GroupVersion.WithResource("forwards")).
		Namespace("default").
		Delete(context.Background(), "test-dns-zone", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	err = wait.Poll(time.Second, time.Second*5, func() (bool, error) {
		_, ok := pluginInstanceMap.Get("crd.test")
		return !ok, nil
	})
	if err != nil {
		t.Fatalf("Expected lookup for crd.test to fail: %s", err)
	}

	if testPluginInstancer.testPluginHandlers[0].OnShutdownCallCount() != 1 {
		t.Fatalf("Expected plugin OnShutdown to have been called once, but got: %d", testPluginInstancer.testPluginHandlers[0].OnShutdownCallCount())
	}

	if err := controller.Stop(); err != nil {
		t.Fatalf("Expected no error: %s", err)
	}
}

func TestForwardLimitNamespace(t *testing.T) {
	controller, client, testPluginInstancer, pluginInstanceMap := setupControllerTestcase(t, "kube-system")
	forward := &corednsv1alpha1.Forward{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dns-zone",
			Namespace: "default",
		},
		Spec: corednsv1alpha1.ForwardSpec{
			From: "crd.test",
			To:   []string{"127.0.0.2"},
		},
	}

	_, err := client.Resource(corednsv1alpha1.GroupVersion.WithResource("forwards")).
		Namespace("default").
		Create(context.Background(), mustForwardToUnstructured(forward), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	kubeSystemForward := &corednsv1alpha1.Forward{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "system-dns-zone",
			Namespace: "kube-system",
		},
		Spec: corednsv1alpha1.ForwardSpec{
			From: "system.test",
			To:   []string{"127.0.0.3"},
		},
	}

	_, err = client.Resource(corednsv1alpha1.GroupVersion.WithResource("forwards")).
		Namespace("kube-system").
		Create(context.Background(), mustForwardToUnstructured(kubeSystemForward), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Expected not to error: %s", err)
	}

	err = wait.Poll(time.Second, time.Second*5, func() (bool, error) {
		return testPluginInstancer.NewWithConfigCallCount() == 1, nil
	})
	if err != nil {
		t.Fatalf("Expected plugin instance to have been called exactly once: %s, plugin instance call count: %d", err, testPluginInstancer.NewWithConfigCallCount())
	}

	handler := testPluginInstancer.NewWithConfigArgsForCall(0)
	if handler.ReceivedConfig.From != "system.test" {
		t.Fatalf("Expected plugin to be created for zone: %s but was: %s", "system.test", handler.ReceivedConfig.From)
	}

	_, ok := pluginInstanceMap.Get("system.test")
	if !ok {
		t.Fatal("Expected plugin lookup to succeed")
	}

	_, ok = pluginInstanceMap.Get("crd.test")
	if ok {
		t.Fatal("Expected plugin lookup to fail")
	}

	if err := controller.Stop(); err != nil {
		t.Fatalf("Expected no error: %s", err)
	}
}

func setupControllerTestcase(t *testing.T, namespace string) (forwardCRDController, *fake.FakeDynamicClient, *TestPluginInstancer, *PluginInstanceMap) {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(corednsv1alpha1.GroupVersion, &corednsv1alpha1.Forward{})
	customListKinds := map[schema.GroupVersionResource]string{
		corednsv1alpha1.GroupVersion.WithResource("forwards"): "ForwardList",
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, customListKinds)
	pluginMap := NewPluginInstanceMap()
	testPluginInstancer := &TestPluginInstancer{}
	controller := newForwardCRDController(context.Background(), client, scheme, namespace, pluginMap, func(cfg forward.ForwardConfig) (lifecyclePluginHandler, error) {
		return testPluginInstancer.NewWithConfig(cfg)
	})

	go controller.Run(1)

	err := wait.Poll(time.Second, time.Second*5, func() (bool, error) {
		return controller.HasSynced(), nil
	})
	if err != nil {
		t.Fatalf("Expected controller to have synced: %s", err)
	}

	return controller, client, testPluginInstancer, pluginMap
}

func mustForwardToUnstructured(forward *corednsv1alpha1.Forward) *unstructured.Unstructured {
	forward.TypeMeta = metav1.TypeMeta{
		Kind:       "Forward",
		APIVersion: "coredns.io/v1alpha1",
	}

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(forward)
	if err != nil {
		panic(fmt.Sprintf("coding error: unable to convert to unstructured: %s", err))
	}
	return &unstructured.Unstructured{
		Object: obj,
	}
}

func mustUnstructuredToForward(obj *unstructured.Unstructured) *corednsv1alpha1.Forward {
	forward := &corednsv1alpha1.Forward{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, forward)
	if err != nil {
		panic(fmt.Sprintf("coding error: unable to convert from unstructured: %s", err))
	}
	return forward
}
