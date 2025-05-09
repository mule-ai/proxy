package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/mule-ai/proxy/pkg/config"
	"github.com/mule-ai/proxy/pkg/metrics"
	"github.com/mule-ai/proxy/pkg/openai"
	"github.com/mule-ai/proxy/pkg/proxy"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize OpenAI client
	openaiClient := openai.NewClient(cfg.OpenAIAPIURL, cfg.OpenAIAPIKey)

	// Initialize metrics collector
	metricsCollector := metrics.NewMetricsCollector(
		cfg.InfluxDBURL,
		cfg.InfluxToken,
		cfg.InfluxOrg,
		cfg.InfluxBucket,
	)
	defer metricsCollector.Close()

	// Create queue manager with OpenAI client
	queueManager := proxy.NewQueueManager(cfg.Endpoints, openaiClient)

	// Create context for shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the priority queue scheduler
	go queueManager.StartScheduler(ctx)

	// Create request handler
	handler := proxy.NewRequestHandler(queueManager)

	// Start HTTP servers for each endpoint
	var servers []*http.Server
	for _, ep := range cfg.Endpoints {
		portStr := fmt.Sprintf(":%d", ep.Port)
		
		mux := http.NewServeMux()
		mux.Handle("/", handler)
		
		server := &http.Server{
			Addr:    portStr,
			Handler: mux,
		}
		
		servers = append(servers, server)
		
		go func(port string) {
			log.Printf("Starting proxy on %s", port)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Server error: %v", err)
			}
		}(portStr)
	}

	log.Println("OpenAI Proxy is running with preemption prioritization")
	
	// Set up graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	
	<-stop
	log.Println("Shutting down servers...")
	
	// Cancel the scheduler context
	cancel()
	
	// Shutdown all servers
	for _, server := range servers {
		if err := server.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down server: %v", err)
		}
	}
	
	log.Println("Servers gracefully stopped")
}