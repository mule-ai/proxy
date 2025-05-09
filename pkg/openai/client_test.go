package openai

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	baseURL := "https://api.openai.com/v1"
	apiKey := "test-key"
	
	client := NewClient(baseURL, apiKey)
	
	if client.BaseURL != baseURL {
		t.Errorf("Expected BaseURL to be %s, got %s", baseURL, client.BaseURL)
	}
	
	if client.APIKey != apiKey {
		t.Errorf("Expected APIKey to be %s, got %s", apiKey, client.APIKey)
	}
	
	if client.HTTPClient == nil {
		t.Error("Expected HTTPClient to be initialized")
	}
}

func TestForwardRequest(t *testing.T) {
	// Set up test server for /v1/chat/completions
	chatServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check request headers
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Expected Authorization header to be 'Bearer test-key', got %s", r.Header.Get("Authorization"))
		}
		
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type header to be 'application/json', got %s", r.Header.Get("Content-Type"))
		}

		// Check request path
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("Expected path to be '/v1/chat/completions', got %s", r.URL.Path)
		}
		
		// Read request body
		var bodyBytes []byte
		var err error
		if r.Body != nil {
			bodyBytes, err = io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("Failed to read request body: %v", err)
			}
		}
		
		// Check request body
		expectedBody := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello"}]}`
		if string(bodyBytes) != expectedBody {
			t.Errorf("Expected request body to be %s, got %s", expectedBody, string(bodyBytes))
		}
		
		// Send response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test-response","choices":[{"message":{"content":"Hello there!"}}]}`))
	}))
	defer chatServer.Close()

	// Set up test server for /v1/models
	modelsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check path
		if r.URL.Path != "/v1/models" {
			t.Errorf("Expected path to be '/v1/models', got %s", r.URL.Path)
		}
		
		// Send response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[{"id":"gpt-4"}, {"id":"gpt-3.5-turbo"}]}`))
	}))
	defer modelsServer.Close()
	
	// Test chat completions endpoint
	client := NewClient(chatServer.URL, "test-key")
	body := bytes.NewBufferString(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello"}]}`)
	
	resp, err := client.ForwardRequest(context.Background(), "POST", "/v1/chat/completions", body)
	if err != nil {
		t.Fatalf("Failed to forward request: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
	
	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	
	expectedResp := `{"id":"test-response","choices":[{"message":{"content":"Hello there!"}}]}`
	if string(respBody) != expectedResp {
		t.Errorf("Expected response body to be %s, got %s", expectedResp, string(respBody))
	}

	// Test leading slash handling
	client = NewClient(chatServer.URL, "test-key")
	body = bytes.NewBufferString(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello"}]}`)
	
	resp, err = client.ForwardRequest(context.Background(), "POST", "v1/chat/completions", body)
	if err != nil {
		t.Fatalf("Failed to forward request: %v", err)
	}
	defer resp.Body.Close()

	// Test models endpoint with nil body
	client = NewClient(modelsServer.URL, "test-key")
	resp, err = client.ForwardRequest(context.Background(), "GET", "/v1/models", nil)
	if err != nil {
		t.Fatalf("Failed to forward request with nil body: %v", err)
	}
	defer resp.Body.Close()
}

func TestExtractRequestMetadata(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		expectedModel  string
		expectedTokens int64
		expectedTools  []string
	}{
		{
			name:           "Chat completions request",
			body:           `{"model":"gpt-4","messages":[{"role":"user","content":"Hello there, how are you today?"}]}`,
			expectedModel:  "gpt-4",
			expectedTokens: 7,
			expectedTools:  nil,
		},
		{
			name:           "Completions request with string prompt",
			body:           `{"model":"davinci","prompt":"Write a poem about AI"}`,
			expectedModel:  "davinci",
			expectedTokens: 5,
			expectedTools:  nil,
		},
		{
			name:           "Completions request with array prompt",
			body:           `{"model":"davinci","prompt":["Write a poem", "about AI"]}`,
			expectedModel:  "davinci",
			expectedTokens: 5,
			expectedTools:  nil,
		},
		{
			name:           "Embeddings request",
			body:           `{"model":"text-embedding-ada-002","input":"The food was delicious and the service was excellent."}`,
			expectedModel:  "text-embedding-ada-002",
			expectedTokens: 13,
			expectedTools:  nil,
		},
		{
			name:           "Embeddings request with array input",
			body:           `{"model":"text-embedding-ada-002","input":["The food was delicious", "and the service was excellent."]}`,
			expectedModel:  "text-embedding-ada-002",
			expectedTokens: 12,
			expectedTools:  nil,
		},
		{
			name:           "Chat completions with tools",
			body:           `{"model":"gpt-4","messages":[{"role":"user","content":"What's the weather?"}],"tools":[{"type":"function","function":{"name":"get_weather"}}]}`,
			expectedModel:  "gpt-4",
			expectedTokens: 4,
			expectedTools:  []string{"function"},
		},
		{
			name:           "Empty request",
			body:           `{}`,
			expectedModel:  "",
			expectedTokens: 0,
			expectedTools:  nil,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(tt.body)
			model, tokens, tools, err := ExtractRequestMetadata(body)
			if err != nil {
				t.Fatalf("Failed to extract request metadata: %v", err)
			}
			
			if model != tt.expectedModel {
				t.Errorf("Expected model to be %s, got %s", tt.expectedModel, model)
			}
			
			if tokens != tt.expectedTokens {
				t.Errorf("Expected tokens to be %d, got %d", tt.expectedTokens, tokens)
			}
			
			if !reflect.DeepEqual(tools, tt.expectedTools) {
				t.Errorf("Expected tools to be %v, got %v", tt.expectedTools, tools)
			}
		})
	}

	// Test with nil body
	model, tokens, tools, err := ExtractRequestMetadata(nil)
	if err != nil {
		t.Fatalf("Failed to extract request metadata for nil body: %v", err)
	}

	if model != "" || tokens != 0 || tools != nil {
		t.Errorf("Expected empty metadata for nil body, got model=%s, tokens=%d, tools=%v", model, tokens, tools)
	}

	// Test with invalid JSON
	invalidBody := strings.NewReader("invalid JSON")
	_, _, _, err = ExtractRequestMetadata(invalidBody)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestRewriteBody(t *testing.T) {
	originalBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	body := strings.NewReader(originalBody)
	
	newBody, err := RewriteBody(body)
	if err != nil {
		t.Fatalf("Failed to rewrite body: %v", err)
	}
	
	bodyBytes, err := io.ReadAll(newBody)
	if err != nil {
		t.Fatalf("Failed to read new body: %v", err)
	}
	
	if string(bodyBytes) != originalBody {
		t.Errorf("Expected new body to be %s, got %s", originalBody, string(bodyBytes))
	}

	// Test with nil body
	nilBody, err := RewriteBody(nil)
	if err != nil {
		t.Fatalf("Failed to rewrite nil body: %v", err)
	}

	if nilBody != nil {
		t.Errorf("Expected nil for nil body rewrite, got %v", nilBody)
	}
}