package rlc

import (
	"github.com/prometheus/client_golang/prometheus"
)

// https://github.com/grafana/groupcache_exporter/blob/main/exporter.go

type Exporter struct {
	rlc *RlcHandler

	cacheItems     *prometheus.Desc
	cacheGets      *prometheus.Desc
	cacheHits      *prometheus.Desc
	cacheEvictions *prometheus.Desc
}

func NewExporter(labels map[string]string, rlc *RlcHandler) *Exporter {
	return &Exporter{
		rlc: rlc,

		cacheItems: prometheus.NewDesc(
			"coredns_rlc_cache_items",
			"Gauge of current items in use",
			[]string{},
			labels,
		),
		cacheGets: prometheus.NewDesc(
			"coredns_rlc_cache_gets_total",
			"Count of cache gets",
			[]string{},
			labels,
		),
		cacheHits: prometheus.NewDesc(
			"coredns_rlc_cache_hits_total",
			"Count of cache hits",
			[]string{},
			labels,
		),
		cacheEvictions: prometheus.NewDesc(
			"coredns_rlc_cache_evictions_total",
			"Count of cache evictions",
			[]string{},
			labels,
		),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {

	ch <- e.cacheItems
	ch <- e.cacheGets
	ch <- e.cacheHits
	ch <- e.cacheEvictions
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.collectCacheStats(ch, e.rlc)
}

func (e *Exporter) collectCacheStats(ch chan<- prometheus.Metric, rlc *RlcHandler) {
	metrics := rlc.cache.Metrics()
	ch <- prometheus.MustNewConstMetric(e.cacheItems, prometheus.GaugeValue, float64(rlc.cache.Len()))
	ch <- prometheus.MustNewConstMetric(e.cacheGets, prometheus.CounterValue, float64(metrics.Hits+metrics.Misses))
	ch <- prometheus.MustNewConstMetric(e.cacheHits, prometheus.CounterValue, float64(metrics.Hits))
	ch <- prometheus.MustNewConstMetric(e.cacheEvictions, prometheus.CounterValue, float64(metrics.Evictions))
}
