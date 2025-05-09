package proxy

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// MockOpenAIClient implements a mock of the OpenAI client interface
type MockOpenAIClient struct {
	ResponseBody    string
	ResponseHeaders map[string]string
	ResponseStatus  int
	RequestDelay    time.Duration
	CallCount       int
	CustomForwarder func(ctx context.Context, method, path string, body io.Reader) (*http.Response, error)
}

// ForwardRequest mocks the OpenAI client's ForwardRequest method
func (m *MockOpenAIClient) ForwardRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	// Use custom implementation if provided
	if m.CustomForwarder != nil {
		return m.CustomForwarder(ctx, method, path, body)
	}

	m.CallCount++
	
	// Simulate processing delay
	if m.RequestDelay > 0 {
		time.Sleep(m.RequestDelay)
	}
	
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		// Continue with the response
	}
	
	// Create a response
	resp := &http.Response{
		StatusCode: m.ResponseStatus,
		Body:       io.NopCloser(strings.NewReader(m.ResponseBody)),
		Header:     make(http.Header),
	}
	
	// Add headers
	for k, v := range m.ResponseHeaders {
		resp.Header.Set(k, v)
	}
	
	return resp, nil
}