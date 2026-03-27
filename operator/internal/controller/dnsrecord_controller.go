package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dnsv1alpha1 "github.com/hanzoai/dns-operator/api/v1alpha1"
)

// DnsRecordReconciler reconciles a DnsRecord object.
type DnsRecordReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dns.hanzo.ai,resources=dnsrecords,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.hanzo.ai,resources=dnsrecords/status,verbs=get;update;patch

func (r *DnsRecordReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var record dnsv1alpha1.DnsRecord
	if err := r.Get(ctx, req.NamespacedName, &record); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Validate the record belongs to an existing zone.
	var zoneList dnsv1alpha1.DnsZoneList
	if err := r.List(ctx, &zoneList, client.InNamespace(req.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	zoneFound := false
	for _, z := range zoneList.Items {
		if z.Name == record.Spec.ZoneRef {
			zoneFound = true
			break
		}
	}

	if !zoneFound {
		logger.Info("zone not found for record", "zoneRef", record.Spec.ZoneRef)
		record.Status.Phase = "Error"
		if err := r.Status().Update(ctx, &record); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	record.Status.Phase = "Active"
	if err := r.Status().Update(ctx, &record); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DnsRecordReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1alpha1.DnsRecord{}).
		Complete(r)
}
