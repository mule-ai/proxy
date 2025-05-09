package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mule-ai/proxy/pkg/config"
	"github.com/mule-ai/proxy/pkg/metrics"
)

func TestNewQueueManager(t *testing.T) {
	// Initialize metrics collector
	metrics.NewMetricsCollector("http://localhost:8086", "test-token", "test-org", "test-bucket")
	endpoints := []config.Endpoint{
		{Port: 8080, Priority: 1, Preemptive: true},
		{Port: 8081, Priority: 2, Preemptive: false},
	}
	
	client := &MockOpenAIClient{
		ResponseBody:   `{"id":"test-response"}`,
		ResponseStatus: 200,
	}
	
	qm := NewQueueManager(endpoints, client)
	
	if len(qm.Queues) != 2 {
		t.Errorf("Expected 2 queues, got %d", len(qm.Queues))
	}
	
	// Test FindQueue
	q1 := qm.FindQueue(1)
	if q1 == nil || q1.Priority != 1 {
		t.Errorf("Expected to find queue with priority 1")
	}
	
	q2 := qm.FindQueue(2)
	if q2 == nil || q2.Priority != 2 {
		t.Errorf("Expected to find queue with priority 2")
	}
	
	q3 := qm.FindQueue(3)
	if q3 != nil {
		t.Errorf("Expected nil for non-existent queue")
	}
	
	// Test FindQueueByPort
	q1ByPort := qm.FindQueueByPort(8080)
	if q1ByPort == nil || q1ByPort.Priority != 1 {
		t.Errorf("Expected to find queue for port 8080")
	}
	
	q2ByPort := qm.FindQueueByPort(8081)
	if q2ByPort == nil || q2ByPort.Priority != 2 {
		t.Errorf("Expected to find queue for port 8081")
	}
	
	qNonExistent := qm.FindQueueByPort(9999)
	if qNonExistent != nil {
		t.Errorf("Expected nil for non-existent port")
	}
}

func TestQueueManagerPreemption(t *testing.T) {
	// Initialize metrics collector
	metrics.NewMetricsCollector("http://localhost:8086", "test-token", "test-org", "test-bucket")
	
	// Create a controlled test environment
	highPriorityQueue := make(chan *workRequest, 1)
	lowPriorityQueue := make(chan *workRequest, 1)
	
	// Create queue manager with manual queues for testing
	qm := &QueueManager{
		Queues: []*PriorityQueue{
			{
				Port:       8080,
				Priority:   1,
				Preemptive: true,
				Requests:   highPriorityQueue,
			},
			{
				Port:       8081,
				Priority:   2,
				Preemptive: false,
				Requests:   lowPriorityQueue,
			},
		},
		OpenAIClient: &MockOpenAIClient{
			ResponseBody:   `{"id":"test-response"}`,
			ResponseStatus: 200,
		},
		mu: sync.RWMutex{},
	}
	
	// Force sort the queues to ensure priority order
	qm.sortByPriority()
	
	// Create a high priority request
	highReq := &workRequest{
		Request:        httptest.NewRequest("POST", "/v1/chat/completions", nil),
		ResponseWriter: httptest.NewRecorder(),
		Done:           make(chan struct{}),
		Model:          "gpt-4",
	}
	
	// Add high priority request to the queue
	highPriorityQueue <- highReq
	
	// Check preemption behavior
	if !qm.ShouldPreempt(2) {
		t.Error("Expected queue 2 to be preemptible by queue 1")
	}
	
	if qm.ShouldPreempt(1) {
		t.Error("Did not expect queue 1 to be preemptible")
	}
	
	// Clean up
	<-highPriorityQueue
	close(highReq.Done)
}

func TestShouldPreempt(t *testing.T) {
	// Initialize metrics collector
	metrics.NewMetricsCollector("http://localhost:8086", "test-token", "test-org", "test-bucket")
	endpoints := []config.Endpoint{
		{Port: 8080, Priority: 1, Preemptive: true},
		{Port: 8081, Priority: 2, Preemptive: false},
		{Port: 8082, Priority: 3, Preemptive: true},
	}
	
	client := &MockOpenAIClient{}
	qm := NewQueueManager(endpoints, client)
	
	// No preemption when queues are empty
	if qm.ShouldPreempt(2) {
		t.Error("Expected no preemption when queues are empty")
	}
	
	// Add request to high priority queue
	q1 := qm.FindQueue(1)
	req := &workRequest{
		Request: &http.Request{},
		Done:    make(chan struct{}),
	}
	q1.Requests <- req
	
	// Now preemption should happen for lower priority queues
	if !qm.ShouldPreempt(2) {
		t.Error("Expected preemption for lower priority when higher priority queue has items")
	}
	
	// But preemption shouldn't happen for higher priority
	if qm.ShouldPreempt(1) {
		t.Error("Expected no preemption for highest priority")
	}
	
	// Consume the request to empty the queue
	<-q1.Requests
	
	// Test with non-preemptive queue
	q2 := qm.FindQueue(2)
	q2.Requests <- req
	
	// No preemption should happen since queue 2 is not preemptive
	if qm.ShouldPreempt(3) {
		t.Error("Expected no preemption from non-preemptive queue")
	}
}

func TestProcessRequestPreemption(t *testing.T) {
	// Initialize metrics collector
	metrics.NewMetricsCollector("http://localhost:8086", "test-token", "test-org", "test-bucket")
	
	// Create a test request
	requestURL := "http://example.com/v1/chat/completions"
	testReq, _ := http.NewRequest("POST", requestURL, bytes.NewBufferString(`{"model":"gpt-4"}`))
	testReq.Header.Set("Content-Type", "application/json")
	
	// Create a recorder for the response
	recorder := httptest.NewRecorder()
	
	// Create the work request
	workReq := &workRequest{
		Request:        testReq,
		ResponseWriter: recorder,
		Done:           make(chan struct{}),
		Model:          "gpt-4",
		InputTokens:    100,
	}
	
	// Create a mock client that delays to allow preemption
	mockClient := &MockOpenAIClient{
		ResponseBody:   `{"id":"test-response"}`,
		ResponseStatus: 200,
		RequestDelay:   100 * time.Millisecond, // Delay to test preemption
	}
	
	// Create a queue manager
	qm := &QueueManager{
		Queues:       []*PriorityQueue{},
		OpenAIClient: mockClient,
		mu:           sync.RWMutex{},
	}
	
	// Create queue
	queue := &PriorityQueue{
		Port:       8080,
		Priority:   2, // Lower priority to test preemption
		Preemptive: false,
		Requests:   make(chan *workRequest, 1),
	}
	
	// Start processing in a goroutine to allow cancellation
	go func() {
		// Process the request
		qm.processRequest(workReq, queue)
	}()
	
	// Wait briefly to let processing start
	time.Sleep(10 * time.Millisecond)
	
	// Manually trigger cancellation to simulate preemption
	if workReq.PreemptCancel != nil {
		workReq.PreemptCancel()
	}
	
	// Wait a bit to complete
	time.Sleep(20 * time.Millisecond)
	
	// Clean up
	close(workReq.Done)
}

func TestProcessRequestWithError(t *testing.T) {
	// Initialize metrics collector
	metrics.NewMetricsCollector("http://localhost:8086", "test-token", "test-org", "test-bucket")
	
	// Create a test request
	requestURL := "http://example.com/v1/chat/completions"
	testReq, _ := http.NewRequest("POST", requestURL, bytes.NewBufferString(`{"model":"gpt-4"}`))
	testReq.Header.Set("Content-Type", "application/json")
	
	// Create a recorder for the response
	recorder := httptest.NewRecorder()
	
	// Create the work request
	workReq := &workRequest{
		Request:        testReq,
		ResponseWriter: recorder,
		Done:           make(chan struct{}),
		Model:          "gpt-4",
		InputTokens:    100,
	}
	
	// Define an error-producing client
	errorClient := &MockOpenAIClient{
		ResponseBody:   "", 
		ResponseStatus: 500,
		// Return nil response to cause an error
		CustomForwarder: func(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
			return nil, fmt.Errorf("test error")
		},
	}
	
	// Create a queue manager with the error client
	qm := &QueueManager{
		Queues:       []*PriorityQueue{},
		OpenAIClient: errorClient,
		mu:           sync.RWMutex{},
	}
	
	// Create queue
	queue := &PriorityQueue{
		Port:       8080,
		Priority:   1,
		Preemptive: false,
		Requests:   make(chan *workRequest, 1),
	}
	
	// Process the request that will fail
	qm.processRequest(workReq, queue)
	
	// Check the response status
	if recorder.Code != http.StatusBadGateway {
		t.Errorf("Expected status code %d, got %d", http.StatusBadGateway, recorder.Code)
	}
	
	// Check if the error message contains the expected text
	if !strings.Contains(recorder.Body.String(), "Error forwarding request") {
		t.Errorf("Expected error message to contain 'Error forwarding request', got: %s", recorder.Body.String())
	}
}

func TestProcessNextRequest(t *testing.T) {
	// Initialize metrics collector
	metrics.NewMetricsCollector("http://localhost:8086", "test-token", "test-org", "test-bucket")
	
	// Create channels for test queues
	highQueue := make(chan *workRequest, 1)
	lowQueue := make(chan *workRequest, 1)
	
	// Create request to test with
	requestURL := "http://example.com/v1/chat/completions"
	testReq, _ := http.NewRequest("POST", requestURL, bytes.NewBufferString(`{"model":"gpt-4"}`))
	
	// Create work request
	workReq := &workRequest{
		Request:        testReq,
		ResponseWriter: httptest.NewRecorder(),
		Done:           make(chan struct{}),
		Model:          "gpt-4",
	}
	
	// Create a mock client that handles the request
	mockClient := &MockOpenAIClient{
		ResponseBody:   `{"id":"test-response"}`,
		ResponseStatus: 200,
		ResponseHeaders: map[string]string{
			"Content-Type": "application/json",
		},
	}
	
	// Create a queue manager with our test queues
	qm := &QueueManager{
		Queues: []*PriorityQueue{
			{
				Port:       8080,
				Priority:   1,
				Preemptive: true,
				Requests:   highQueue,
			},
			{
				Port:       8081,
				Priority:   2,
				Preemptive: false,
				Requests:   lowQueue,
			},
		},
		OpenAIClient: mockClient,
		mu:          sync.RWMutex{},
	}
	
	// Add the request to the high priority queue
	highQueue <- workReq
	
	// Process just one request
	go qm.processNextRequest()
	
	// Wait for the process to finish
	select {
	case <-workReq.Done:
		// Expected - request completed
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Test timed out")
	}
}

func TestStartScheduler(t *testing.T) {
	// Initialize metrics collector
	metrics.NewMetricsCollector("http://localhost:8086", "test-token", "test-org", "test-bucket")
	
	mockClient := &MockOpenAIClient{
		ResponseBody:   `{"id":"test-response"}`,
		ResponseStatus: 200,
	}
	
	requestQueue := make(chan *workRequest, 1)
	
	qm := &QueueManager{
		Queues: []*PriorityQueue{
			{
				Port:       8080,
				Priority:   1,
				Preemptive: true,
				Requests:   requestQueue,
			},
		},
		OpenAIClient: mockClient,
		mu:          sync.RWMutex{},
	}
	
	// Start the scheduler with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	
	// Run scheduler in background
	go qm.StartScheduler(ctx)
	
	// Wait a bit for scheduler to start
	time.Sleep(20 * time.Millisecond)
	
	// Create a request to process
	req := &workRequest{
		Request:        httptest.NewRequest("POST", "/v1/test", nil),
		ResponseWriter: httptest.NewRecorder(),
		Done:           make(chan struct{}),
		Model:          "gpt-4",
	}
	
	// Send request to queue
	requestQueue <- req
	
	// Wait for the request to be processed
	select {
	case <-req.Done:
		// Request was processed
	case <-time.After(100 * time.Millisecond):
		// This is acceptable for this test since we're just testing scheduler cancellation
	}
	
	// Cancel the context to stop the scheduler
	cancel()
	
	// Wait a bit for the scheduler to stop
	time.Sleep(20 * time.Millisecond)
	
	// Check that stopping flag was set
	if !qm.stopping {
		t.Error("Expected stopping to be true after context cancellation")
	}
}