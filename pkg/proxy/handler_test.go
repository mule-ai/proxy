package proxy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mule-ai/proxy/pkg/config"
	"github.com/mule-ai/proxy/pkg/metrics"
)

func TestHandlerServeHTTP(t *testing.T) {
	// Initialize metrics collector
	metrics.NewMetricsCollector("http://localhost:8086", "test-token", "test-org", "test-bucket")

	// Create a mock client for testing
	client := &MockOpenAIClient{
		ResponseBody:   `{"id":"test-response","choices":[{"message":{"content":"Hello there!"}}]}`,
		ResponseStatus: 200,
		ResponseHeaders: map[string]string{
			"Content-Type": "application/json",
		},
	}

	// Configure endpoints for the test
	endpoints := []config.Endpoint{
		{Port: 8080, Priority: 1, Preemptive: true},
		{Port: 8081, Priority: 2, Preemptive: false},
	}

	// Create queue manager with mock client
	qm := NewQueueManager(endpoints, client)

	// Start the scheduler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go qm.StartScheduler(ctx)

	// Create handler
	handler := NewRequestHandler(qm)

	// Test handling a chat completions request
	chatReqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(chatReqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")
	req.Host = "localhost:8080" // Set port to match our configured endpoint

	// Record the response
	recorder := httptest.NewRecorder()

	// Serve the request
	handler.ServeHTTP(recorder, req)

	// Verify the response
	resp := recorder.Result()
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type to be application/json, got %s", resp.Header.Get("Content-Type"))
	}

	// Test OPTIONS request for CORS
	optReq := httptest.NewRequest("OPTIONS", "/v1/chat/completions", nil)
	optReq.Host = "localhost:8080"
	optRecorder := httptest.NewRecorder()
	handler.ServeHTTP(optRecorder, optReq)

	if optRecorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d for OPTIONS, got %d", http.StatusOK, optRecorder.Code)
	}

	if optRecorder.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin header to be '*'")
	}

	// Test unsupported method
	putReq := httptest.NewRequest("PUT", "/v1/chat/completions", nil)
	putReq.Host = "localhost:8080"
	putRecorder := httptest.NewRecorder()
	handler.ServeHTTP(putRecorder, putReq)

	if putRecorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status code %d for PUT, got %d", http.StatusMethodNotAllowed, putRecorder.Code)
	}

	// Test unknown port
	unknownReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	unknownReq.Host = "localhost:9999" // Port that doesn't exist in our config
	unknownRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unknownRecorder, unknownReq)

	if unknownRecorder.Code != http.StatusNotFound {
		t.Errorf("Expected status code %d for unknown port, got %d", http.StatusNotFound, unknownRecorder.Code)
	}

	// Test invalid port
	invalidReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	invalidReq.Host = "localhost:invalid" // Non-numeric port
	invalidRecorder := httptest.NewRecorder()
	handler.ServeHTTP(invalidRecorder, invalidReq)

	if invalidRecorder.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d for invalid port, got %d", http.StatusBadRequest, invalidRecorder.Code)
	}
}

func TestHandlerWithFullQueue(t *testing.T) {
	// Initialize metrics collector
	metrics.NewMetricsCollector("http://localhost:8086", "test-token", "test-org", "test-bucket")
	
	// Create a mock client with delay to ensure queue fills up
	client := &MockOpenAIClient{
		ResponseBody:   `{"id":"test-response"}`,
		ResponseStatus: 200,
		RequestDelay:   100 * time.Millisecond,
	}
	
	// Create a mutex to protect the requests channel
	mu := sync.Mutex{}
	
	// Create a channel with only 1 capacity
	requests := make(chan *workRequest, 1)
	
	// Create queue manager with mock client for testing
	qm := &QueueManager{
		Queues: []*PriorityQueue{
			{
				Port:       8080,
				Priority:   1,
				Preemptive: true,
				Requests:   requests,
			},
		},
		OpenAIClient: client,
		mu:          sync.RWMutex{},
	}
	
	// Fill up the queue manually
	req := &workRequest{
		Request:        httptest.NewRequest("POST", "/v1/test", nil),
		ResponseWriter: httptest.NewRecorder(),
		Done:           make(chan struct{}),
	}
	
	// Add a request to the queue and block it
	mu.Lock()
	requests <- req
	mu.Unlock()
	
	// Create handler
	handler := NewRequestHandler(qm)
	
	// Try a request which should fail with queue full
	testReq := httptest.NewRequest("POST", "/v1/chat/completions", 
		bytes.NewBufferString(`{"model":"gpt-4","messages":[{"role":"user","content":"Test request"}]}`))
	testReq.Host = "localhost:8080"
	recorder := httptest.NewRecorder()
	
	// Handle the request - this should fail with 429
	handler.ServeHTTP(recorder, testReq)
	
	// Check that we got a 429 Too Many Requests
	if recorder.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status code %d for full queue, got %d", http.StatusTooManyRequests, recorder.Code)
	}
	
	// Check the error message
	if !strings.Contains(recorder.Body.String(), "Service overloaded") {
		t.Errorf("Expected error message about service being overloaded, got: %s", recorder.Body.String())
	}
	
	// Clean up
	close(req.Done)
}