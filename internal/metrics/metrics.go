package metrics

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests partitioned by method, route and status code.",
	}, []string{"method", "route", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	HTTPRequestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Current number of HTTP requests being processed.",
	})
)

// PoolCollector reports pgxpool connection stats to Prometheus.
type PoolCollector struct {
	pool              *pgxpool.Pool
	totalConns        *prometheus.Desc
	idleConns         *prometheus.Desc
	acquiredConns     *prometheus.Desc
	constructingConns *prometheus.Desc
	maxConns          *prometheus.Desc
}

func NewPoolCollector(pool *pgxpool.Pool) *PoolCollector {
	return &PoolCollector{
		pool: pool,
		totalConns: prometheus.NewDesc(
			"db_pool_total_conns",
			"Total number of connections in the pool.",
			nil, nil,
		),
		idleConns: prometheus.NewDesc(
			"db_pool_idle_conns",
			"Number of idle connections in the pool.",
			nil, nil,
		),
		acquiredConns: prometheus.NewDesc(
			"db_pool_acquired_conns",
			"Number of acquired (in-use) connections in the pool.",
			nil, nil,
		),
		constructingConns: prometheus.NewDesc(
			"db_pool_constructing_conns",
			"Number of connections currently being constructed.",
			nil, nil,
		),
		maxConns: prometheus.NewDesc(
			"db_pool_max_conns",
			"Maximum number of connections allowed in the pool.",
			nil, nil,
		),
	}
}

func (c *PoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalConns
	ch <- c.idleConns
	ch <- c.acquiredConns
	ch <- c.constructingConns
	ch <- c.maxConns
}

func (c *PoolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.totalConns, prometheus.GaugeValue, float64(s.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.idleConns, prometheus.GaugeValue, float64(s.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.acquiredConns, prometheus.GaugeValue, float64(s.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.constructingConns, prometheus.GaugeValue, float64(s.ConstructingConns()))
	ch <- prometheus.MustNewConstMetric(c.maxConns, prometheus.GaugeValue, float64(s.MaxConns()))
}
