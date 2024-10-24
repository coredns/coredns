package rlc

import (
	"github.com/golang/groupcache"
	"github.com/prometheus/client_golang/prometheus"
)

// https://github.com/grafana/groupcache_exporter/blob/main/exporter.go

type GroupcacheExporter struct {
	groups []*groupcache.Group

	groupGets           *prometheus.Desc
	groupCacheHits      *prometheus.Desc
	groupPeerLoads      *prometheus.Desc
	groupPeerErrors     *prometheus.Desc
	groupLoads          *prometheus.Desc
	groupLoadsDeduped   *prometheus.Desc
	groupLocalLoads     *prometheus.Desc
	groupLocalLoadErrs  *prometheus.Desc
	groupServerRequests *prometheus.Desc
	cacheBytes          *prometheus.Desc
	cacheItems          *prometheus.Desc
	cacheGets           *prometheus.Desc
	cacheHits           *prometheus.Desc
	cacheEvictions      *prometheus.Desc
}

func NewGroupcacheExporter(labels map[string]string, groups ...*groupcache.Group) *GroupcacheExporter {
	return &GroupcacheExporter{
		groups: groups,
		groupGets: prometheus.NewDesc(
			"coredns_rlc_groupcache_gets_total",
			"Counter of cache gets (including from peers)",
			[]string{"group"},
			labels,
		),
		groupCacheHits: prometheus.NewDesc(
			"coredns_rlc_groupcache_hits_total",
			"Count of cache hits (from either main or hot cache)",
			[]string{"group"},
			labels,
		),
		groupPeerLoads: prometheus.NewDesc(
			"coredns_rlc_groupcache_peer_loads_total",
			"Count of non-error loads or cache hits from peers",
			[]string{"group"},
			labels,
		),
		groupPeerErrors: prometheus.NewDesc(
			"coredns_rlc_groupcache_peer_errors_total",
			"Count of errors from peers",
			[]string{"group"},
			labels,
		),
		groupLoads: prometheus.NewDesc(
			"coredns_rlc_groupcache_loads_total",
			"Count of (gets - hits)",
			[]string{"group"},
			labels,
		),
		groupLoadsDeduped: prometheus.NewDesc(
			"coredns_rlc_groupcache_loads_deduped_total",
			"Count of loads after singleflight",
			[]string{"group"},
			labels,
		),
		groupLocalLoads: prometheus.NewDesc(
			"coredns_rlc_groupcache_local_load_total",
			"Count of loads from local cache",
			[]string{"group"},
			labels,
		),
		groupLocalLoadErrs: prometheus.NewDesc(
			"coredns_rlc_groupcache_local_load_errs_total",
			"Count of loads from local cache that failed",
			[]string{"group"},
			labels,
		),
		groupServerRequests: prometheus.NewDesc(
			"coredns_rlc_groupcache_server_requests_total",
			"Count of gets that came over the network from peers",
			[]string{"group"},
			labels,
		),
		cacheBytes: prometheus.NewDesc(
			"coredns_rlc_groupcache_cache_bytes",
			"Gauge of current bytes in use",
			[]string{"group", "type"},
			labels,
		),
		cacheItems: prometheus.NewDesc(
			"coredns_rlc_groupcache_cache_items",
			"Gauge of current items in use",
			[]string{"group", "type"},
			labels,
		),
		cacheGets: prometheus.NewDesc(
			"coredns_rlc_groupcache_cache_gets_total",
			"Count of cache gets",
			[]string{"group", "type"},
			labels,
		),
		cacheHits: prometheus.NewDesc(
			"coredns_rlc_groupcache_cache_hits_total",
			"Count of cache hits",
			[]string{"group", "type"},
			labels,
		),
		cacheEvictions: prometheus.NewDesc(
			"coredns_rlc_groupcache_cache_evictions_total",
			"Count of cache evictions",
			[]string{"group", "type"},
			labels,
		),
	}
}

func (e *GroupcacheExporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.groupGets
	ch <- e.groupCacheHits
	ch <- e.groupPeerLoads
	ch <- e.groupPeerErrors
	ch <- e.groupLoads
	ch <- e.groupLoadsDeduped
	ch <- e.groupLocalLoads
	ch <- e.groupLocalLoadErrs
	ch <- e.groupServerRequests
	ch <- e.cacheBytes
	ch <- e.cacheItems
	ch <- e.cacheGets
	ch <- e.cacheHits
	ch <- e.cacheEvictions
}

func (e *GroupcacheExporter) Collect(ch chan<- prometheus.Metric) {
	for _, group := range e.groups {
		e.collectFromGroup(ch, group)
	}
}

func (e *GroupcacheExporter) collectFromGroup(ch chan<- prometheus.Metric, grp *groupcache.Group) {
	e.collectStats(ch, grp)
	e.collectCacheStats(ch, grp)
}

func (e *GroupcacheExporter) collectStats(ch chan<- prometheus.Metric, grp *groupcache.Group) {
	name := "coredns_rlc_groupcache"
	stats := grp.Stats
	ch <- prometheus.MustNewConstMetric(e.groupGets, prometheus.CounterValue, float64(stats.Gets), name)
	ch <- prometheus.MustNewConstMetric(e.groupCacheHits, prometheus.CounterValue, float64(stats.CacheHits), name)
	ch <- prometheus.MustNewConstMetric(e.groupPeerLoads, prometheus.CounterValue, float64(stats.PeerLoads), name)
	ch <- prometheus.MustNewConstMetric(e.groupPeerErrors, prometheus.CounterValue, float64(stats.PeerErrors), name)
	ch <- prometheus.MustNewConstMetric(e.groupLoads, prometheus.CounterValue, float64(stats.Loads), name)
	ch <- prometheus.MustNewConstMetric(e.groupLoadsDeduped, prometheus.CounterValue, float64(stats.LoadsDeduped), name)
	ch <- prometheus.MustNewConstMetric(e.groupLocalLoads, prometheus.CounterValue, float64(stats.LocalLoads), name)
	ch <- prometheus.MustNewConstMetric(e.groupLocalLoadErrs, prometheus.CounterValue, float64(stats.LocalLoadErrs), name)
	ch <- prometheus.MustNewConstMetric(e.groupServerRequests, prometheus.CounterValue, float64(stats.ServerRequests), name)
}

func (e *GroupcacheExporter) collectCacheStats(ch chan<- prometheus.Metric, grp *groupcache.Group) {
	name := "coredns_rlc_groupcache"
	stats := grp.CacheStats(groupcache.MainCache)
	ch <- prometheus.MustNewConstMetric(e.cacheItems, prometheus.GaugeValue, float64(stats.Items), name, "main")
	ch <- prometheus.MustNewConstMetric(e.cacheBytes, prometheus.GaugeValue, float64(stats.Bytes), name, "main")
	ch <- prometheus.MustNewConstMetric(e.cacheGets, prometheus.CounterValue, float64(stats.Gets), name, "main")
	ch <- prometheus.MustNewConstMetric(e.cacheHits, prometheus.CounterValue, float64(stats.Hits), name, "main")
	ch <- prometheus.MustNewConstMetric(e.cacheEvictions, prometheus.CounterValue, float64(stats.Evictions), name, "main")

	stats = grp.CacheStats(groupcache.HotCache)
	ch <- prometheus.MustNewConstMetric(e.cacheItems, prometheus.GaugeValue, float64(stats.Items), name, "hot")
	ch <- prometheus.MustNewConstMetric(e.cacheBytes, prometheus.GaugeValue, float64(stats.Bytes), name, "hot")
	ch <- prometheus.MustNewConstMetric(e.cacheGets, prometheus.CounterValue, float64(stats.Gets), name, "hot")
	ch <- prometheus.MustNewConstMetric(e.cacheHits, prometheus.CounterValue, float64(stats.Hits), name, "hot")
	ch <- prometheus.MustNewConstMetric(e.cacheEvictions, prometheus.CounterValue, float64(stats.Evictions), name, "hot")
}
