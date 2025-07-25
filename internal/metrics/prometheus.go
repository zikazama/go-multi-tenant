package metrics

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"strconv"
	"time"
)

var (
	// HTTP metrics
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	// Tenant metrics
	activeTenants = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_tenants_total",
			Help: "Total number of active tenants",
		},
	)

	// Message metrics
	messagesProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "messages_processed_total",
			Help: "Total number of messages processed",
		},
		[]string{"tenant_id", "status"},
	)

	messageQueueDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "message_queue_depth",
			Help: "Current depth of message queues",
		},
		[]string{"tenant_id"},
	)

	// Worker metrics
	activeWorkers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "active_workers_total",
			Help: "Number of active workers per tenant",
		},
		[]string{"tenant_id"},
	)
)

func init() {
	// Register metrics
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(activeTenants)
	prometheus.MustRegister(messagesProcessed)
	prometheus.MustRegister(messageQueueDepth)
	prometheus.MustRegister(activeWorkers)
}

// PrometheusMiddleware creates a Gin middleware for Prometheus metrics
func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		httpRequestsTotal.WithLabelValues(c.Request.Method, c.FullPath(), status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, c.FullPath()).Observe(duration)
	}
}

// MetricsHandler returns the Prometheus metrics handler
func MetricsHandler() gin.HandlerFunc {
	return gin.WrapH(promhttp.Handler())
}

// Metric update functions
func IncrementActiveTenants() {
	activeTenants.Inc()
}

func DecrementActiveTenants() {
	activeTenants.Dec()
}

func IncrementMessagesProcessed(tenantID, status string) {
	messagesProcessed.WithLabelValues(tenantID, status).Inc()
}

func SetMessageQueueDepth(tenantID string, depth float64) {
	messageQueueDepth.WithLabelValues(tenantID).Set(depth)
}

func SetActiveWorkers(tenantID string, workers float64) {
	activeWorkers.WithLabelValues(tenantID).Set(workers)
}