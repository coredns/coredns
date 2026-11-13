package object

import (
	"testing"

	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestToPodTerminalPhaseDropsIPs(t *testing.T) {
	tests := []struct {
		phase   api.PodPhase
		wantIPs int
	}{
		{api.PodPending, 1},
		{api.PodRunning, 1},
		{api.PodSucceeded, 0},
		{api.PodFailed, 0},
	}
	for _, tc := range tests {
		t.Run(string(tc.phase), func(t *testing.T) {
			apiPod := &api.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "ns"},
				Status: api.PodStatus{
					Phase:  tc.phase,
					PodIPs: []api.PodIP{{IP: "10.0.0.1"}},
				},
			}
			obj, err := ToPod(apiPod)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			pod := obj.(*Pod)
			if len(pod.PodIPs) != tc.wantIPs {
				t.Errorf("expected %d pod IPs, got %v", tc.wantIPs, pod.PodIPs)
			}
		})
	}
}
