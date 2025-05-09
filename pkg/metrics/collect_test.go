package metrics

import (
	"sync"
	"testing"
	"time"
)

// TestCollectMethod tests the Collect method of the MetricsCollector
func TestCollectMethod(t *testing.T) {
	// Reset singleton for testing
	collector = nil
	once = sync.Once{}

	// Create a metrics collector with test collection function
	var collectCount int
	m := &MetricsCollector{
		mu: sync.Mutex{},
		CollectFn: func(metrics RequestMetrics) error {
			collectCount++
			return nil
		},
	}

	// Set as the singleton collector
	collector = m

	// Create sample metrics
	metrics := RequestMetrics{
		Model:          "gpt-4",
		InputTokens:    100,
		ProcessingTime: 500 * time.Millisecond,
		RetryCount:     2,
		Tools:          []string{"function", "retrieval"},
		EndpointPath:   "/v1/chat/completions",
		Priority:       1,
		Preempted:      true,
		StatusCode:     200,
	}

	// Collect metrics
	err := m.Collect(metrics)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify that the collection function was called
	if collectCount != 1 {
		t.Fatalf("Expected 1 collection call, got %d", collectCount)
	}

	// Test with nil tools
	metrics.Tools = nil
	err = m.Collect(metrics)
	if err != nil {
		t.Fatalf("Expected no error with nil tools, got %v", err)
	}

	// Verify that the collection function was called again
	if collectCount != 2 {
		t.Fatalf("Expected 2 collection calls, got %d", collectCount)
	}
}

// TestDefaultCollectFn tests the defaultCollectFn function
func TestDefaultCollectFn(t *testing.T) {
	// Create sample metrics
	metrics := RequestMetrics{
		Model:          "gpt-4",
		InputTokens:    100,
		ProcessingTime: 500 * time.Millisecond,
		RetryCount:     2,
		Tools:          []string{"function", "retrieval"},
		EndpointPath:   "/v1/chat/completions",
		Priority:       1,
		Preempted:      true,
		StatusCode:     200,
	}

	// Test the default collect function
	err := defaultCollectFn(metrics)
	if err != nil {
		t.Fatalf("Expected no error from defaultCollectFn, got %v", err)
	}

	// Test with nil tools
	metrics.Tools = nil
	err = defaultCollectFn(metrics)
	if err != nil {
		t.Fatalf("Expected no error from defaultCollectFn with nil tools, got %v", err)
	}
}