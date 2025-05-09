package main

import (
	"os"
	"testing"
	
	"github.com/mule-ai/proxy/pkg/config"
)

// TestConfigLoading tests config loading from main
func TestConfigLoading(t *testing.T) {
	// Create a temporary config file
	tmpfile, err := os.CreateTemp("", "config-test-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	configData := `{
		"influxdb_url": "http://localhost:8086",
		"influx_token": "test-token",
		"influx_org": "test-org",
		"influx_bucket": "test-bucket",
		"openai_api_url": "https://api.openai.com/v1",
		"openai_api_key": "test-key",
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

	if _, err := tmpfile.Write([]byte(configData)); err != nil {
		t.Fatalf("Failed to write config data: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Test loading the config
	cfg, err := config.LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify config values
	if cfg.InfluxDBURL != "http://localhost:8086" {
		t.Errorf("Expected InfluxDBURL to be 'http://localhost:8086', got '%s'", cfg.InfluxDBURL)
	}

	if len(cfg.Endpoints) != 2 {
		t.Fatalf("Expected 2 endpoints, got %d", len(cfg.Endpoints))
	}

	if cfg.Endpoints[0].Priority != 1 || !cfg.Endpoints[0].Preemptive {
		t.Errorf("Endpoint 0 has incorrect values")
	}

	if cfg.Endpoints[1].Priority != 2 || cfg.Endpoints[1].Preemptive {
		t.Errorf("Endpoint 1 has incorrect values")
	}
}