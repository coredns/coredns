package kubernetes

import (
	"context"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

func serviceWatchFunc(ctx context.Context, c kubernetes.Interface, ns string, s labels.Selector) func(options meta.ListOptions) (watch.Interface, error) {
	return func(options meta.ListOptions) (watch.Interface, error) {
		if s != nil {
			options.LabelSelector = s.String()
		}
		w, err := c.CoreV1().Services(ns).Watch(ctx, options)
		return w, err
	}
}

func podWatchFunc(ctx context.Context, c kubernetes.Interface, ns string, s labels.Selector) func(options meta.ListOptions) (watch.Interface, error) {
	return func(options meta.ListOptions) (watch.Interface, error) {
		if s != nil {
			options.LabelSelector = s.String()
		}
		if len(options.FieldSelector) > 0 {
			options.FieldSelector = options.FieldSelector + ","
		}
		options.FieldSelector = options.FieldSelector + "status.phase!=Succeeded,status.phase!=Failed,status.phase!=Unknown"
		w, err := c.CoreV1().Pods(ns).Watch(ctx, options)
		return w, err
	}
}

func endpointsWatchFunc(ctx context.Context, c kubernetes.Interface, ns string, s labels.Selector) func(options meta.ListOptions) (watch.Interface, error) {
	return func(options meta.ListOptions) (watch.Interface, error) {
		if s != nil {
			options.LabelSelector = s.String()
		}
		w, err := c.CoreV1().Endpoints(ns).Watch(ctx, options)
		return w, err
	}
}

func namespaceWatchFunc(ctx context.Context, c kubernetes.Interface, s labels.Selector) func(options meta.ListOptions) (watch.Interface, error) {
	return func(options meta.ListOptions) (watch.Interface, error) {
		if s != nil {
			options.LabelSelector = s.String()
		}
		w, err := c.CoreV1().Namespaces().Watch(ctx, options)
		return w, err
	}
}
