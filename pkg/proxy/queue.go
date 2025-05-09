package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/mule-ai/proxy/pkg/config"
	"github.com/mule-ai/proxy/pkg/metrics"
)

// OpenAIClient defines the interface for an OpenAI API client
type OpenAIClient interface {
	ForwardRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error)
}

// PriorityQueue represents a queue for requests with specific priority
type PriorityQueue struct {
	Port       int
	Priority   int      // Lower number = higher priority (1 is top)
	Preemptive bool     // Whether this queue can preempt lower-priority ones
	Requests   chan *workRequest
}

// workRequest encapsulates a single request and its state
type workRequest struct {
	Request           *http.Request
	ResponseWriter    http.ResponseWriter
	Done              chan struct{}
	PreemptCtx        context.Context
	PreemptCancel     context.CancelFunc
	StartTime         time.Time
	Model             string
	InputTokens       int64
	ProcessingTime    time.Duration
	Tools             []string
	RetryCount        int
	Preempted         bool
}

// QueueManager manages all priority queues
type QueueManager struct {
	Queues      []*PriorityQueue
	OpenAIClient OpenAIClient
	mu          sync.RWMutex
	stopping    bool
}

// NewQueueManager creates a new queue manager with specified priority queues
func NewQueueManager(endpoints []config.Endpoint, openaiClient OpenAIClient) *QueueManager {
	queues := make([]*PriorityQueue, 0, len(endpoints))
	for _, ep := range endpoints {
		queues = append(queues, &PriorityQueue{
			Port:       ep.Port,
			Priority:   ep.Priority,
			Preemptive: ep.Preemptive,
			Requests:   make(chan *workRequest, 100),
		})
	}
	
	return &QueueManager{
		Queues:      queues,
		OpenAIClient: openaiClient,
	}
}

// FindQueue gets a queue by priority level
func (qm *QueueManager) FindQueue(priority int) *PriorityQueue {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	
	for _, q := range qm.Queues {
		if q.Priority == priority {
			return q
		}
	}
	return nil
}

// FindQueueByPort gets a queue by port number
func (qm *QueueManager) FindQueueByPort(port int) *PriorityQueue {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	
	for _, q := range qm.Queues {
		if q.Port == port {
			return q
		}
	}
	return nil
}

// Sort queues by priority (ascending)
func (qm *QueueManager) sortByPriority() {
	sort.Slice(qm.Queues, func(i, j int) bool {
		return qm.Queues[i].Priority < qm.Queues[j].Priority
	})
}

// StartScheduler begins the queue processing and preemption logic
func (qm *QueueManager) StartScheduler(ctx context.Context) {
	qm.sortByPriority()
	
	for {
		select {
		case <-ctx.Done():
			qm.stopping = true
			// Wait for all queues to drain
			return
		default:
			// Process the highest priority queue with requests
			qm.processNextRequest()
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// processNextRequest finds and processes the highest priority request
func (qm *QueueManager) processNextRequest() {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	
	// Find the highest priority queue with requests
	var activeQueue *PriorityQueue
	for _, q := range qm.Queues {
		select {
		case req := <-q.Requests:
			// Found a request in this queue
			activeQueue = q
			
			// Process the request
			go qm.processRequest(req, activeQueue)
			return
		default:
			// Queue is empty, try the next one
			continue
		}
	}
}

// ShouldPreempt checks if a higher priority preemptive queue has requests
func (qm *QueueManager) ShouldPreempt(currentPriority int) bool {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	
	if qm.stopping {
		return false
	}
	
	// Check all higher priority queues that are preemptive
	for _, q := range qm.Queues {
		if q.Priority < currentPriority && q.Preemptive && len(q.Requests) > 0 {
			return true
		}
	}
	return false
}

// processRequest handles a single work request and ensures retry on preemption
func (qm *QueueManager) processRequest(req *workRequest, queue *PriorityQueue) {
	// Create a new context for this request that can be cancelled for preemption
	ctx, cancel := context.WithCancel(context.Background())
	req.PreemptCtx = ctx
	req.PreemptCancel = cancel
	
	// Start a goroutine to monitor for preemption
	go func() {
		for {
			select {
			case <-req.Done:
				// Request completed normally
				return
			case <-time.After(50 * time.Millisecond):
				// Check for preemption periodically
				if qm.ShouldPreempt(queue.Priority) {
					// Cancel the current request
					cancel()
					
					// Only requeue if this is a lower priority queue
					if queue.Priority > 1 {
						// Mark as preempted for metrics
						req.Preempted = true
						req.RetryCount++
						
						// Create a new request object since the old one is being used
						newReq := &workRequest{
							Request:        req.Request.Clone(context.Background()),
							ResponseWriter: req.ResponseWriter,
							Done:           req.Done,
							StartTime:      req.StartTime,
							Model:          req.Model,
							InputTokens:    req.InputTokens,
							Tools:          req.Tools,
							RetryCount:     req.RetryCount,
							Preempted:      req.Preempted,
						}
						
						// Send to its queue for retry
						select {
						case queue.Requests <- newReq:
							fmt.Printf("Preempted request for model %s, priority %d. Retrying (attempt %d)\n", 
								req.Model, queue.Priority, req.RetryCount+1)
						default:
							// Queue is full, this shouldn't happen but handle it
							fmt.Printf("ERROR: Could not requeue preempted request, queue is full\n")
							
							// Write error response
							req.ResponseWriter.WriteHeader(http.StatusServiceUnavailable)
							req.ResponseWriter.Write([]byte(`{"error":"Service overloaded, please try again later"}`))
							close(req.Done)
						}
					}
					return
				}
			}
		}
	}()
	
	// Clone the request with our cancellation context
	httpReq := req.Request.Clone(ctx)
	
	// Forward the request to OpenAI
	startTime := time.Now()
	resp, err := qm.OpenAIClient.ForwardRequest(ctx, httpReq.Method, httpReq.URL.Path, httpReq.Body)
	processingTime := time.Since(startTime)
	
	// Check if the request was cancelled due to preemption
	select {
	case <-ctx.Done():
		// Request was preempted, we'll retry
		return
	default:
		// Request completed, process the response
		if err != nil {
			req.ResponseWriter.WriteHeader(http.StatusBadGateway)
			req.ResponseWriter.Write([]byte(fmt.Sprintf(`{"error":"Error forwarding request: %v"}`, err)))
			close(req.Done)
			return
		}
		
		// Copy headers from OpenAI response
		for k, v := range resp.Header {
			for _, vv := range v {
				req.ResponseWriter.Header().Add(k, vv)
			}
		}
		
		// Set status code
		req.ResponseWriter.WriteHeader(resp.StatusCode)
		
		// Copy body
		_, err = io.Copy(req.ResponseWriter, resp.Body)
		resp.Body.Close()
		
		if err != nil {
			fmt.Printf("Error copying response body: %v\n", err)
		}
		
		// Record metrics
		metricsCollector := metrics.GetCollector()
		if metricsCollector != nil {
			metricsCollector.Collect(metrics.RequestMetrics{
				Model:          req.Model,
				InputTokens:    req.InputTokens,
				ProcessingTime: processingTime,
				RetryCount:     req.RetryCount,
				Tools:          req.Tools,
				EndpointPath:   req.Request.URL.Path,
				Priority:       queue.Priority,
				Preempted:      req.Preempted,
				StatusCode:     resp.StatusCode,
			})
		}
		
		fmt.Printf("Completed request for model: %s (Path: %s, Priority: %d, Preemptions: %d, Time: %v)\n", 
			req.Model, req.Request.URL.Path, queue.Priority, req.RetryCount, processingTime)
		
		// Signal that the request is done
		close(req.Done)
	}
}