package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	testConfig := `{
	  "influxdb_url": "http://test-influx:8086",
	  "influx_token": "test-token",
	  "influx_org": "test-org",
	  "influx_bucket": "test-bucket",
	  "openai_api_url": "https://test-api.openai.com/v1",
	  "openai_api_key": "test-key-123",
	  "endpoints": [
	    {
	      "port": 8080,
	      "priority": 1,
	      "preemptive": true
	    },
	    {
	      "port": 8081,
	      "priority": 2,
	      "preemptive": false
	    }
	  ]
	}`

	tmpfile, err := os.CreateTemp("", "config-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(testConfig)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Test loading the config
	cfg, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify config values
	if cfg.InfluxDBURL != "http://test-influx:8086" {
		t.Errorf("Expected InfluxDBURL to be 'http://test-influx:8086', got '%s'", cfg.InfluxDBURL)
	}

	if cfg.OpenAIAPIURL != "https://test-api.openai.com/v1" {
		t.Errorf("Expected OpenAIAPIURL to be 'https://test-api.openai.com/v1', got '%s'", cfg.OpenAIAPIURL)
	}

	if len(cfg.Endpoints) != 2 {
		t.Fatalf("Expected 2 endpoints, got %d", len(cfg.Endpoints))
	}

	// Check first endpoint
	if cfg.Endpoints[0].Port != 8080 || cfg.Endpoints[0].Priority != 1 || !cfg.Endpoints[0].Preemptive {
		t.Errorf("First endpoint doesn't match expected values")
	}

	// Check second endpoint
	if cfg.Endpoints[1].Port != 8081 || cfg.Endpoints[1].Priority != 2 || cfg.Endpoints[1].Preemptive {
		t.Errorf("Second endpoint doesn't match expected values")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	// Create a minimal config file to test defaults
	testConfig := `{
	  "influxdb_url": "http://test-influx:8086",
	  "influx_token": "test-token",
	  "endpoints": []
	}`

	tmpfile, err := os.CreateTemp("", "config-defaults-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(testConfig)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Test loading the config
	cfg, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify default values
	if cfg.OpenAIAPIURL != "https://api.openai.com/v1" {
		t.Errorf("Expected default OpenAIAPIURL to be 'https://api.openai.com/v1', got '%s'", cfg.OpenAIAPIURL)
	}

	if cfg.InfluxBucket != "proxybucket" {
		t.Errorf("Expected default InfluxBucket to be 'proxybucket', got '%s'", cfg.InfluxBucket)
	}

	if cfg.InfluxOrg != "openaiorg" {
		t.Errorf("Expected default InfluxOrg to be 'openaiorg', got '%s'", cfg.InfluxOrg)
	}
}

func TestLoadConfigError(t *testing.T) {
	// Test loading non-existent file
	_, err := LoadConfig("non-existent-file.json")
	if err == nil {
		t.Error("Expected error when loading non-existent file, got nil")
	}

	// Test loading invalid JSON
	tmpfile, err := os.CreateTemp("", "config-invalid-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte("invalid JSON")); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	_, err = LoadConfig(tmpfile.Name())
	if err == nil {
		t.Error("Expected error when loading invalid JSON, got nil")
	}
}