# OpenAI API Proxy with Preemption

A high-performance proxy for the OpenAI API that provides preemptive prioritization for requests.

## Features

- **Priority Queuing**: Assign different priorities to different endpoints
- **Preemptive Scheduling**: Higher priority requests preempt lower priority ones
- **Transparent Retry**: Preempted requests are automatically retried
- **Metrics Collection**: Detailed request metrics sent to InfluxDB
- **Multiple Ports**: Each port represents a different priority level

## Configuration

The proxy is configured via a `config.json` file:

```json
{
  "influxdb_url": "http://localhost:8086",
  "influx_token": "your-influx-token",
  "influx_org": "openaiorg",
  "influx_bucket": "proxybucket",
  "openai_api_url": "https://api.openai.com/v1",
  "openai_api_key": "your-openai-api-key",
  "endpoints": [
    {
      "port": 8080,
      "priority": 1,
      "preemptive": true
    },
    {
      "port": 8081,
      "priority": 2,
      "preemptive": true
    },
    {
      "port": 8082,
      "priority": 3,
      "preemptive": false
    }
  ]
}
```

### Configuration Parameters

- `influxdb_url`: URL of your InfluxDB instance
- `influx_token`: Authentication token for InfluxDB
- `influx_org`: Organization name in InfluxDB
- `influx_bucket`: Bucket name for metrics in InfluxDB
- `openai_api_url`: Base URL of the OpenAI API
- `openai_api_key`: Your OpenAI API key
- `endpoints`: Array of endpoint configurations:
  - `port`: Port to listen on for this endpoint (each port represents a different priority)
  - `priority`: Priority level (lower number = higher priority)
  - `preemptive`: Whether requests on this port can preempt lower priority ones

## Usage

1. Configure your `config.json` file
2. Run the proxy:
   ```
   go run cmd/main.go
   ```
3. Send OpenAI API requests to the configured ports (each port serves all OpenAI API endpoints)
   ```
   # High priority request
   curl -X POST http://localhost:8080/v1/chat/completions \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer $OPENAI_API_KEY" \
     -d '{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}'
   
   # Medium priority request
   curl -X POST http://localhost:8081/v1/completions \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer $OPENAI_API_KEY" \
     -d '{"model": "text-davinci-003", "prompt": "Hello"}'
   ```

## Metrics

The proxy collects and sends the following metrics to InfluxDB:

- Model being requested
- Input token count (estimated)
- Processing time
- Number of retries due to preemption
- API endpoint path
- Queue priority level
- Whether the request was preempted
- HTTP status code of the response
- Tools requested in the API call (if any)

## Development

### Prerequisites

- Go 1.24 or later
- InfluxDB (for metrics collection)

### Building

```
go build -o openai-proxy cmd/main.go
```

### Running Tests

```
go test ./... -v
```

## Architecture

The proxy is built around a port-based priority queue system:

1. Each port has its own priority queue
2. Higher priority queues (lower port numbers) are always processed before lower priority ones
3. All ports serve the full OpenAI API (chat completions, completions, embeddings, etc.)
4. Preemptive queues can interrupt processing of lower priority requests
5. Interrupted requests are automatically requeued and retried transparently

## License

[MIT License](LICENSE)