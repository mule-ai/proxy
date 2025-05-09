package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client handles communication with the OpenAI API
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewClient creates a new OpenAI API client
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 300 * time.Second, // 5-minute timeout for long-running requests
		},
	}
}

// ForwardRequest forwards a request to the OpenAI API and returns the response
func (c *Client) ForwardRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	// Construct full URL
	url := c.BaseURL
	// Ensure path is properly formatted with leading slash
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	
	url += path

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	// Make request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request to OpenAI API: %w", err)
	}

	return resp, nil
}

// ExtractRequestMetadata extracts model name, token count and other metadata for metrics
func ExtractRequestMetadata(body io.Reader) (string, int64, []string, error) {
	if body == nil {
		return "", 0, nil, nil
	}

	// Read the entire body
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return "", 0, nil, err
	}

	// Create a new reader with the same content for further use
	bodyReader := bytes.NewReader(bodyBytes)

	// Parse the body as JSON
	var request map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &request); err != nil {
		return "", 0, nil, err
	}

	// Extract model name
	model, _ := request["model"].(string)

	// Estimate token count based on input
	var inputTokens int64 = 0

	// Handle different request types
	if messages, ok := request["messages"].([]interface{}); ok {
		// Chat completions request
		for _, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				if content, ok := msgMap["content"].(string); ok {
					// Rough estimation: 1 token ≈ 4 characters
					inputTokens += int64(len(content) / 4)
				}
			}
		}
	} else if prompt, ok := request["prompt"].(string); ok {
		// Completions request
		inputTokens += int64(len(prompt) / 4)
	} else if promptArray, ok := request["prompt"].([]interface{}); ok {
		// Completions request with array prompt
		for _, p := range promptArray {
			if promptStr, ok := p.(string); ok {
				inputTokens += int64(len(promptStr) / 4)
			}
		}
	} else if input, ok := request["input"].(string); ok {
		// Embeddings request
		inputTokens += int64(len(input) / 4)
	} else if inputArray, ok := request["input"].([]interface{}); ok {
		// Embeddings request with array input
		for _, i := range inputArray {
			if inputStr, ok := i.(string); ok {
				inputTokens += int64(len(inputStr) / 4)
			}
		}
	}

	// Extract tools if present
	var tools []string
	if toolsArray, ok := request["tools"].([]interface{}); ok {
		for _, tool := range toolsArray {
			if toolMap, ok := tool.(map[string]interface{}); ok {
				if toolType, ok := toolMap["type"].(string); ok {
					tools = append(tools, toolType)
				}
			}
		}
	}

	// Reset reader position for further use
	_, err = bodyReader.Seek(0, io.SeekStart)
	if err != nil {
		return model, inputTokens, tools, err
	}

	return model, inputTokens, tools, nil
}

// RewriteBody creates a new reader with the same content as the original
func RewriteBody(body io.Reader) (io.Reader, error) {
	if body == nil {
		return nil, nil
	}

	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	
	return bytes.NewReader(bodyBytes), nil
}