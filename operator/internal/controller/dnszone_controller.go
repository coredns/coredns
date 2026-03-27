package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dnsv1alpha1 "github.com/hanzoai/dns-operator/api/v1alpha1"
)

// DnsZoneReconciler reconciles a DnsZone object.
type DnsZoneReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	HTTPClient *http.Client
}

// +kubebuilder:rbac:groups=dns.hanzo.ai,resources=dnszones,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.hanzo.ai,resources=dnszones/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.hanzo.ai,resources=dnsrecords,verbs=get;list;watch

func (r *DnsZoneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the DnsZone.
	var zone dnsv1alpha1.DnsZone
	if err := r.Get(ctx, req.NamespacedName, &zone); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// List all DnsRecords for this zone.
	var recordList dnsv1alpha1.DnsRecordList
	if err := r.List(ctx, &recordList, client.InNamespace(req.Namespace)); err != nil {
		logger.Error(err, "unable to list DnsRecords")
		return ctrl.Result{}, err
	}

	// Filter records belonging to this zone.
	var zoneRecords []dnsv1alpha1.DnsRecord
	for _, rec := range recordList.Items {
		if rec.Spec.ZoneRef == zone.Name {
			zoneRecords = append(zoneRecords, rec)
		}
	}

	// Update record count in status.
	zone.Status.RecordCount = len(zoneRecords)

	// Sync to CoreDNS if enabled.
	if zone.Spec.SyncToCoreDNS {
		if err := r.syncToCoreDNS(ctx, &zone, zoneRecords); err != nil {
			logger.Error(err, "failed to sync to CoreDNS")
			zone.Status.Phase = "Error"
			if updateErr := r.Status().Update(ctx, &zone); updateErr != nil {
				logger.Error(updateErr, "failed to update zone status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		now := metav1.Now()
		zone.Status.LastSyncTime = &now
	}

	zone.Status.Phase = "Active"
	if err := r.Status().Update(ctx, &zone); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

// syncToCoreDNS pushes zone records to the CoreDNS hanzodns /api/v1/sync endpoint.
func (r *DnsZoneReconciler) syncToCoreDNS(ctx context.Context, zone *dnsv1alpha1.DnsZone, records []dnsv1alpha1.DnsRecord) error {
	type syncRecord struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		TTL      int    `json:"ttl"`
		Content  string `json:"content"`
		Priority int    `json:"priority,omitempty"`
		Proxied  bool   `json:"proxied"`
	}

	type syncZone struct {
		Zone    string       `json:"zone"`
		OrgID   string       `json:"org_id,omitempty"`
		Records []syncRecord `json:"records"`
	}

	syncRecords := make([]syncRecord, 0, len(records))
	for _, rec := range records {
		sr := syncRecord{
			Name:    rec.Spec.Name,
			Type:    rec.Spec.Type,
			TTL:     rec.Spec.TTL,
			Content: rec.Spec.Content,
			Proxied: rec.Spec.Proxied,
		}
		if rec.Spec.Priority != nil {
			sr.Priority = *rec.Spec.Priority
		}
		syncRecords = append(syncRecords, sr)
	}

	payload := map[string]interface{}{
		"zones": []syncZone{{
			Zone:    zone.Spec.Zone,
			OrgID:   zone.Spec.OrgID,
			Records: syncRecords,
		}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal sync payload: %w", err)
	}

	endpoint := zone.Spec.CoreDNSEndpoint
	if endpoint == "" {
		endpoint = "http://coredns-hanzodns.dns-system.svc:8443"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/api/v1/sync", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := r.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sync request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sync returned status %d", resp.StatusCode)
	}

	return nil
}

func (r *DnsZoneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1alpha1.DnsZone{}).
		Complete(r)
}
