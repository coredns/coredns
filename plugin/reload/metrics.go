package reload

import (
	"encoding/hex"
	"sync"

	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics for the reload plugin
var (
	// failedCount is the counter of the number of failed reload attempts.
	failedCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: "reload",
		Name:      "failed_total",
		Help:      "Counter of the number of failed reload attempts.",
	})
	// reloadInfo reports the hashes of running configurations.
	reloadInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "reload",
		Name:      "version_info",
		Help:      "A metric with a constant '1' value labeled by hash, and value which type of hash generated.",
	}, []string{"hash", "value"})
)

var activeVersions = struct {
	sync.Mutex
	instances map[*reload]struct{}
}{instances: make(map[*reload]struct{})}

func (r *reload) publishVersion() {
	activeVersions.Lock()
	defer activeVersions.Unlock()
	if shutdownRequested(r.quit) {
		return
	}
	activeVersions.instances[r] = struct{}{}
	reloadInfo.WithLabelValues("sha512", hex.EncodeToString(r.configHash[:])).Set(1)
}

func (r *reload) removeVersion() {
	activeVersions.Lock()
	defer activeVersions.Unlock()
	if _, published := activeVersions.instances[r]; !published {
		return
	}
	delete(activeVersions.instances, r)
	for instance := range activeVersions.instances {
		if instance.configHash == r.configHash {
			return
		}
	}
	reloadInfo.DeleteLabelValues("sha512", hex.EncodeToString(r.configHash[:]))
}
