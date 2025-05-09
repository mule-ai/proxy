package config

import (
	"encoding/json"
	"os"
)

// Config represents the application configuration
type Config struct {
	InfluxDBURL string     `json:"influxdb_url"`
	InfluxToken string     `json:"influx_token"`
	InfluxOrg   string     `json:"influx_org"`
	InfluxBucket string    `json:"influx_bucket"`
	OpenAIAPIURL string    `json:"openai_api_url"`
	OpenAIAPIKey string    `json:"openai_api_key"`
	Endpoints   []Endpoint `json:"endpoints"`
}

// Endpoint represents a priority endpoint configuration
type Endpoint struct {
	Port       int    `json:"port"`
	Priority   int    `json:"priority"`
	Preemptive bool   `json:"preemptive"`
}

// LoadConfig loads the configuration from a file
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	// Set defaults if not specified
	if config.OpenAIAPIURL == "" {
		config.OpenAIAPIURL = "https://api.openai.com/v1"
	}
	
	if config.InfluxBucket == "" {
		config.InfluxBucket = "proxybucket"
	}
	
	if config.InfluxOrg == "" {
		config.InfluxOrg = "openaiorg"
	}

	return &config, nil
}