package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dnsv1alpha1 "github.com/hanzoai/dns-operator/api/v1alpha1"
)

// CloudflareReconciler syncs DnsRecords with syncToCloudflare=true to Cloudflare.
type CloudflareReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// CFAPIToken is the Cloudflare API token.
	CFAPIToken string
}

// +kubebuilder:rbac:groups=dns.hanzo.ai,resources=dnsrecords,verbs=get;list;watch
// +kubebuilder:rbac:groups=dns.hanzo.ai,resources=dnsrecords/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.hanzo.ai,resources=dnszones,verbs=get;list;watch

func (r *CloudflareReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Only process records that need Cloudflare sync.
	var record dnsv1alpha1.DnsRecord
	if err := r.Get(ctx, req.NamespacedName, &record); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !record.Spec.SyncToCloudflare {
		return ctrl.Result{}, nil
	}

	// Find the parent zone to get the Cloudflare zone ID.
	var zoneList dnsv1alpha1.DnsZoneList
	if err := r.List(ctx, &zoneList, client.InNamespace(req.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	var parentZone *dnsv1alpha1.DnsZone
	for i := range zoneList.Items {
		if zoneList.Items[i].Name == record.Spec.ZoneRef {
			parentZone = &zoneList.Items[i]
			break
		}
	}

	if parentZone == nil || parentZone.Spec.CloudflareZoneID == "" {
		logger.Info("skipping CF sync: no parent zone or missing cloudflareZoneId",
			"record", record.Name, "zoneRef", record.Spec.ZoneRef)
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// TODO(phase3): Implement actual Cloudflare API calls.
	// For now, log intent and mark as synced.
	logger.Info("would sync to Cloudflare",
		"zone", parentZone.Spec.Zone,
		"cfZoneId", parentZone.Spec.CloudflareZoneID,
		"record", record.Spec.Name,
		"type", record.Spec.Type,
		"content", record.Spec.Content)

	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *CloudflareReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1alpha1.DnsRecord{}).
		Complete(r)
}
