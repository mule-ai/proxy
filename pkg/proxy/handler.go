package proxy

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mule-ai/proxy/pkg/openai"
)

// RequestHandler handles incoming HTTP requests and routes them to the appropriate queue
type RequestHandler struct {
	QueueManager *QueueManager
}

// NewRequestHandler creates a new request handler
func NewRequestHandler(qm *QueueManager) *RequestHandler {
	return &RequestHandler{
		QueueManager: qm,
	}
}

// ServeHTTP implements the http.Handler interface
func (h *RequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Handle OPTIONS requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow POST and GET for OpenAI API
	if r.Method != "POST" && r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(`{"error":"Method not allowed"}`))
		return
	}

	// Extract the port from the server address
	portStr := strings.TrimPrefix(r.Host, "localhost:")
	portStr = strings.TrimPrefix(portStr, "127.0.0.1:")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"Invalid port"}`))
		return
	}

	// Find the queue for this port
	queue := h.QueueManager.FindQueueByPort(port)
	if queue == nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"No queue configured for this port"}`))
		return
	}

	// Read request body for metrics extraction without consuming it
	var bodyBytes []byte
	var model string
	var inputTokens int64
	var tools []string

	if r.Body != nil {
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"Failed to read request body"}`))
			return
		}
		r.Body.Close()

		// Extract metrics data
		model, inputTokens, tools, err = openai.ExtractRequestMetadata(bytes.NewReader(bodyBytes))
		if err != nil {
			// Just log the error, don't fail the request
			println("Failed to extract request metadata:", err.Error())
		}

		// Restore body for the upcoming request
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Create a done channel to signal completion
	done := make(chan struct{})

	// Create work request
	req := &workRequest{
		Request:        r,
		ResponseWriter: w,
		Done:           done,
		StartTime:      time.Now(),
		Model:          model,
		InputTokens:    inputTokens,
		Tools:          tools,
		RetryCount:     0,
		Preempted:      false,
	}

	// Send to appropriate queue
	select {
	case queue.Requests <- req:
		// Request queued successfully
	default:
		// Queue is full
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"Service overloaded, please try again later"}`))
		return
	}

	// Wait for the request to complete
	<-done
}