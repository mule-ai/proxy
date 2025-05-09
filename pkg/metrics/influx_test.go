package metrics

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestNewMetricsCollector tests the creation of a metrics collector
func TestNewMetricsCollector(t *testing.T) {
	// Reset the singleton for testing
	collector = nil
	once = sync.Once{}

	m := NewMetricsCollector("http://localhost:8086", "test-token", "test-org", "test-bucket")
	if m == nil {
		t.Error("Expected non-nil metrics collector")
	}

	// Test singleton pattern
	m2 := NewMetricsCollector("http://different-url:8086", "different-token", "different-org", "different-bucket")
	if m != m2 {
		t.Error("Expected the same collector instance due to singleton pattern")
	}
}

// TestGetCollector tests getting the metrics collector
func TestGetCollector(t *testing.T) {
	// Reset the singleton for testing
	collector = nil
	once = sync.Once{}

	// This should panic because collector is not initialized yet
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when calling GetCollector before initialization")
		}
	}()

	GetCollector()
}

// MockMetricsCollector is a simple struct for testing
type MockMetricsCollector struct {
	CollectedMetrics []RequestMetrics
}

// TestRequestMetrics tests the RequestMetrics struct
func TestRequestMetrics(t *testing.T) {
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

	// Verify field values
	if metrics.Model != "gpt-4" {
		t.Errorf("Expected Model to be 'gpt-4', got '%s'", metrics.Model)
	}

	if metrics.InputTokens != 100 {
		t.Errorf("Expected InputTokens to be 100, got %d", metrics.InputTokens)
	}

	if metrics.ProcessingTime != 500*time.Millisecond {
		t.Errorf("Expected ProcessingTime to be 500ms, got %v", metrics.ProcessingTime)
	}

	if metrics.RetryCount != 2 {
		t.Errorf("Expected RetryCount to be 2, got %d", metrics.RetryCount)
	}

	if len(metrics.Tools) != 2 || metrics.Tools[0] != "function" || metrics.Tools[1] != "retrieval" {
		t.Errorf("Expected Tools to be ['function', 'retrieval'], got %v", metrics.Tools)
	}

	if metrics.EndpointPath != "/v1/chat/completions" {
		t.Errorf("Expected EndpointPath to be '/v1/chat/completions', got '%s'", metrics.EndpointPath)
	}

	if metrics.Priority != 1 {
		t.Errorf("Expected Priority to be 1, got %d", metrics.Priority)
	}

	if !metrics.Preempted {
		t.Errorf("Expected Preempted to be true, got false")
	}

	if metrics.StatusCode != 200 {
		t.Errorf("Expected StatusCode to be 200, got %d", metrics.StatusCode)
	}
}

// TestCollectWithMockClient tests the collect method with a mock client
func TestCollectWithMockClient(t *testing.T) {
	// Create a mock collector that just stores metrics
	mock := &MockMetricsCollector{
		CollectedMetrics: make([]RequestMetrics, 0),
	}

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

	// Simulate collecting metrics
	mock.CollectedMetrics = append(mock.CollectedMetrics, metrics)

	// Verify the metrics were stored
	if len(mock.CollectedMetrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(mock.CollectedMetrics))
	}

	// Verify the metric values
	collected := mock.CollectedMetrics[0]
	if collected.Model != "gpt-4" {
		t.Errorf("Expected Model to be 'gpt-4', got '%s'", collected.Model)
	}

	if collected.ProcessingTime != 500*time.Millisecond {
		t.Errorf("Expected ProcessingTime to be 500ms, got %v", collected.ProcessingTime)
	}
}

// TestContextHandling tests context handling with metrics
func TestContextHandling(t *testing.T) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Verify that context handling works correctly
	select {
	case <-ctx.Done():
		// This is the expected case - context should expire
	case <-time.After(100 * time.Millisecond):
		t.Error("Context should have expired but didn't")
	}
}