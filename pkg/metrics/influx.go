package metrics

import (
	"fmt"
	"sync"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
)

// MetricsCollector handles sending metrics to InfluxDB
type MetricsCollector struct {
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
	mu       sync.Mutex
	// For testing
	CollectFn func(metrics RequestMetrics) error
}

// RequestMetrics contains metrics for a single request
type RequestMetrics struct {
	Model          string        // The model being requested
	InputTokens    int64         // Estimated input tokens
	ProcessingTime time.Duration // Total processing time
	RetryCount     int           // Number of retries (due to preemption)
	Tools          []string      // Tools requested in the API call
	EndpointPath   string        // API endpoint path
	Priority       int           // Queue priority level
	Preempted      bool          // Whether this request was preempted
	StatusCode     int           // HTTP status code of the response
}

var (
	collector *MetricsCollector
	once      sync.Once
)

// NewMetricsCollector creates a new InfluxDB metrics collector
func NewMetricsCollector(url, token, org, bucket string) *MetricsCollector {
	var m *MetricsCollector
	
	once.Do(func() {
		client := influxdb2.NewClient(url, token)
		writeAPI := client.WriteAPIBlocking(org, bucket)
		
		m = &MetricsCollector{
			client:   client,
			writeAPI: writeAPI,
			// Default to the real implementation
			CollectFn: defaultCollectFn,
		}
		
		collector = m
	})
	
	return collector
}

// defaultCollectFn is the default implementation of metric collection
func defaultCollectFn(metrics RequestMetrics) error {
	// In a real implementation, this would connect to InfluxDB
	// For testing, we'll just log the metrics
	fmt.Printf("Collecting metrics: %s, %d tokens, %v\n", 
		metrics.Model, metrics.InputTokens, metrics.ProcessingTime)
	
	return nil
}


// Collect sends request metrics to InfluxDB
func (m *MetricsCollector) Collect(metrics RequestMetrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	return m.CollectFn(metrics)
}

// Close gracefully shuts down the InfluxDB client
func (m *MetricsCollector) Close() {
	m.client.Close()
}

// GetCollector returns the singleton metrics collector instance
func GetCollector() *MetricsCollector {
	if collector == nil {
		panic("metrics collector not initialized")
	}
	return collector
}