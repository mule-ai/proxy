package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	
	"github.com/mule-ai/proxy/pkg/metrics"
)

// TestQueuePreemption tests that a request in a lower priority queue
// will be preempted by a request in a higher priority queue

// TestQueueFullOnRequeue tests the scenario where a preempted request cannot be requeued
// because the queue is full
func TestQueuePreemption(t *testing.T) {
	// Initialize metrics
	metrics.NewMetricsCollector("http://localhost:8086", "test-token", "test-org", "test-bucket")
	
	// Create a slow mock client to simulate a long-running request
	mockClient := &MockOpenAIClient{
		ResponseBody:   `{"id":"test-response"}`,
		ResponseStatus: 200,
		// Create a custom forwarder that takes some time to complete and detects cancellation
		CustomForwarder: func(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
			// Simulate a long-running request
			select {
			case <-ctx.Done():
				// Request was cancelled
				return nil, ctx.Err()
			case <-time.After(300 * time.Millisecond):
				// Return a success response if not cancelled
				resp := &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"id":"test-response"}`)),
					Header:     make(http.Header),
				}
				return resp, nil
			}
		},
	}
	
	// Create a low priority queue
	lowPriorityQueue := &PriorityQueue{
		Port:       8080,
		Priority:   2, // Lower priority
		Preemptive: false,
		Requests:   make(chan *workRequest, 10),
	}
	
	// Create a high priority queue
	highPriorityQueue := &PriorityQueue{
		Port:       8081,
		Priority:   1, // Higher priority
		Preemptive: true, // Can preempt other queues
		Requests:   make(chan *workRequest, 10),
	}
	
	// Create queue manager
	qm := &QueueManager{
		Queues:       []*PriorityQueue{highPriorityQueue, lowPriorityQueue},
		OpenAIClient: mockClient,
	}
	
	// Channel to detect if a request was requeued
	requeueCh := make(chan bool, 1)
	
	// Channel to detect if preemption check is running
	preemptCheckRunning := make(chan bool, 1)
	
	// Create a low priority request
	lowPriorityReq, _ := http.NewRequest("POST", "/v1/chat/completions", 
		bytes.NewBufferString(`{"model":"gpt-4", "messages":[{"role":"user","content":"test"}]}`))
	lowPriorityReq.Header.Set("Content-Type", "application/json")
	
	// Count requeues
	requeueCount := 0
	
	// Monitor the low priority queue for requests
	go func() {
		// Wait for the second request to appear in the queue (the requeued one)
		req := <-lowPriorityQueue.Requests
		
		// Keep track of this request and see if it gets requeued
		originalReq := req
		
		// Process the request, which should start the preemption monitoring
		go func() {
			preemptCheckRunning <- true
			qm.processRequest(req, lowPriorityQueue)
		}()
		
		// Wait for any requeued requests
		for {
			select {
			case req := <-lowPriorityQueue.Requests:
				requeueCount++
				// Check if this is our original request being requeued
				if req.Model == originalReq.Model && req.Preempted {
					requeueCh <- true
					return
				}
			case <-time.After(500 * time.Millisecond):
				// No requeue detected in time
				requeueCh <- false
				return
			}
		}
	}()
	
	// Submit a low priority request
	lowPriorityQueue.Requests <- &workRequest{
		Request:        lowPriorityReq,
		ResponseWriter: httptest.NewRecorder(),
		Done:           make(chan struct{}),
		Model:          "gpt-4",
		InputTokens:    100,
		StartTime:      time.Now(),
	}
	
	// Wait for the preemption check to start running
	<-preemptCheckRunning
	
	// Wait a bit to let the request start processing
	time.Sleep(50 * time.Millisecond)
	
	// Now submit a high priority request to trigger preemption
	highPriorityReq, _ := http.NewRequest("POST", "/v1/chat/completions", 
		bytes.NewBufferString(`{"model":"gpt-4", "messages":[{"role":"user","content":"high priority"}]}`))
	highPriorityReq.Header.Set("Content-Type", "application/json")
	
	highPriorityQueue.Requests <- &workRequest{
		Request:        highPriorityReq,
		ResponseWriter: httptest.NewRecorder(),
		Done:           make(chan struct{}),
		Model:          "gpt-4",
		InputTokens:    200,
		StartTime:      time.Now(),
	}
	
	// Verify preemption is detected
	if !qm.ShouldPreempt(lowPriorityQueue.Priority) {
		t.Error("Failed to detect need for preemption")
	}
	
	// Wait for requeue to be detected
	wasRequeued := <-requeueCh
	if !wasRequeued {
		t.Error("Request was not requeued after preemption")
	}
	
	// Verify requeue count
	if requeueCount == 0 {
		t.Error("No requeues were detected")
	}
}

// TestQueueFullOnRequeue tests that error handling works correctly when
// a preempted request cannot be requeued because the queue is full
func TestQueueFullOnRequeue(t *testing.T) {
	// Initialize metrics
	metrics.NewMetricsCollector("http://localhost:8086", "test-token", "test-org", "test-bucket")
	
	// Create a mock client that always takes a long time to respond to help with preemption
	mockClient := &MockOpenAIClient{
		ResponseBody:   `{"id":"test-response"}`,
		ResponseStatus: 200,
		CustomForwarder: func(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
			// Simulate a long-running request that checks for cancellation
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(300 * time.Millisecond):
				// Return a success response if not cancelled
				resp := &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"id":"test-response"}`)),
					Header:     make(http.Header),
				}
				return resp, nil
			}
		},
	}
	
	// Create a low priority queue with a very small buffer size so it fills quickly
	lowPriorityQueue := &PriorityQueue{
		Port:       8080,
		Priority:   2, // Lower priority
		Preemptive: false,
		Requests:   make(chan *workRequest, 1), // Tiny buffer to ensure it fills up
	}
	
	// Create a high priority queue
	highPriorityQueue := &PriorityQueue{
		Port:       8081,
		Priority:   1, // Higher priority
		Preemptive: true, // Can preempt other queues
		Requests:   make(chan *workRequest, 10),
	}
	
	// Create queue manager
	qm := &QueueManager{
		Queues:       []*PriorityQueue{highPriorityQueue, lowPriorityQueue},
		OpenAIClient: mockClient,
	}
	
	// Create a request that will be processed and potentially preempted
	testReq, _ := http.NewRequest("POST", "/v1/chat/completions", 
		bytes.NewBufferString(`{"model":"gpt-4", "messages":[{"role":"user","content":"test"}]}`))
	testReq.Header.Set("Content-Type", "application/json")
	
	// Create a recorder to capture the response
	recorder := httptest.NewRecorder()
	
	// Create the work request
	workReq := &workRequest{
		Request:        testReq,
		ResponseWriter: recorder,
		Done:           make(chan struct{}),
		Model:          "gpt-4",
		InputTokens:    100,
		StartTime:      time.Now(),
	}
	
	// Manually start processing the request
	go qm.processRequest(workReq, lowPriorityQueue)
	
	// Give the request time to start processing
	time.Sleep(50 * time.Millisecond)
	
	// Fill the queue so there's no room for a requeued request
	fillerReq, _ := http.NewRequest("POST", "/v1/chat/completions", 
		bytes.NewBufferString(`{"model":"gpt-4", "messages":[{"role":"user","content":"filler"}]}`))
	
	lowPriorityQueue.Requests <- &workRequest{
		Request:        fillerReq,
		ResponseWriter: httptest.NewRecorder(),
		Done:           make(chan struct{}),
		Model:          "gpt-4-filler",
		InputTokens:    100,
		StartTime:      time.Now(),
	}
	
	// Verify queue is full
	isFull := false
	select {
	case lowPriorityQueue.Requests <- &workRequest{}:
		// If this succeeds, queue is not full
	default:
		isFull = true
	}
	
	if !isFull {
		t.Error("Queue should be full for this test")
		return
	}
	
	// Add a high priority request to trigger preemption
	highPriorityReq, _ := http.NewRequest("POST", "/v1/chat/completions", 
		bytes.NewBufferString(`{"model":"gpt-4", "messages":[{"role":"user","content":"high priority"}]}`))
	
	highPriorityQueue.Requests <- &workRequest{
		Request:        highPriorityReq,
		ResponseWriter: httptest.NewRecorder(),
		Done:           make(chan struct{}),
		Model:          "gpt-4-high",
		InputTokens:    200,
		StartTime:      time.Now(),
	}
	
	// Verify that preemption should happen
	if !qm.ShouldPreempt(lowPriorityQueue.Priority) {
		t.Error("ShouldPreempt returned false when it should be true")
	}
	
	// Wait for response to be written (error case when queue is full)
	time.Sleep(300 * time.Millisecond)
	
	// Verify that an error response was written to the recorder
	response := recorder.Result()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, response.StatusCode)
	}
	
	// Check that it contains the expected error message
	bodyBytes, _ := io.ReadAll(response.Body)
	bodyString := string(bodyBytes)
	if bodyString != `{"error":"Service overloaded, please try again later"}` {
		t.Errorf("Unexpected response body: %s", bodyString)
	}
}