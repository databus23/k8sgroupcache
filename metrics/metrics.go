package metrics

import (
	"fmt"

	"github.com/mailgun/groupcache/v2"
	"github.com/prometheus/client_golang/prometheus"
)

func NewGroupCollector(group *groupcache.Group) prometheus.Collector {
	return &collector{group: group}
}

type collector struct {
	group *groupcache.Group
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	prefix := fmt.Sprintf("groupcache_%s_", c.group.Name())

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"gets_total", "Total number of get requests, including from peers", nil, nil),
		prometheus.CounterValue,
		float64(c.group.Stats.Gets.Get()),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"cache_hits_total", "Total number of cache hits", nil, nil),
		prometheus.CounterValue,
		float64(c.group.Stats.CacheHits.Get()),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"peer_load_max_latency_seconds", "Longest time to load a value from peers", nil, nil),
		prometheus.GaugeValue,
		float64(c.group.Stats.GetFromPeersLatencyLower.Get())/1000,
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"peer_loads_total", "Total number of remote load or remote cache hit (not an error)", nil, nil),
		prometheus.CounterValue,
		float64(c.group.Stats.PeerLoads.Get()),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"peer_errors_total", "Total number of errors loading from peers", nil, nil),
		prometheus.CounterValue,
		float64(c.group.Stats.PeerErrors.Get()),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"loads_total", "Total number of gets - cacheHits", nil, nil),
		prometheus.CounterValue,
		float64(c.group.Stats.Loads.Get()),
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"loads_deduped_total", "Total number of whatever", nil, nil),
		prometheus.CounterValue,
		float64(c.group.Stats.LoadsDeduped.Get()),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"local_loads_total", "Total number of loading values locally", nil, nil),
		prometheus.CounterValue,
		float64(c.group.Stats.LocalLoads.Get()),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"local_load_errors_total", "Total number of errors loading values locally", nil, nil),
		prometheus.CounterValue,
		float64(c.group.Stats.LocalLoadErrs.Get()),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"server_requests_total", "Total number of gets that came over the network from peers", nil, nil),
		prometheus.CounterValue,
		float64(c.group.Stats.LocalLoadErrs.Get()),
	)

	cacheStats(ch, prefix+"main_cache_", c.group.CacheStats(groupcache.MainCache))
	cacheStats(ch, prefix+"hot_cache_", c.group.CacheStats(groupcache.HotCache))

}

func cacheStats(ch chan<- prometheus.Metric, prefix string, stats groupcache.CacheStats) {
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"size_bytes", "Cache size", nil, nil),
		prometheus.GaugeValue,
		float64(stats.Bytes),
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"items", "Number of items in the cache", nil, nil),
		prometheus.GaugeValue,
		float64(stats.Items),
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"evictions_total", "Number of evictions", nil, nil),
		prometheus.CounterValue,
		float64(stats.Evictions),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"gets_total", "Number of gets", nil, nil),
		prometheus.CounterValue,
		float64(stats.Gets),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(prefix+"hits_total", "Number of hits", nil, nil),
		prometheus.CounterValue,
		float64(stats.Hits),
	)
}
