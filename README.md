# GPU Reporter

Monitoring API for vLLM, Ollama, and NVIDIA GPU metrics. Auto-detects running inference services and exposes a single JSON endpoint for remote querying.

## Features

- **Auto-detection**: Scans host processes to find vLLM and Ollama services — no manual config needed
- **Unified API**: Single `GET /status` returns all services + GPU metrics in one JSON response
- **GPU monitoring**: Temperature, VRAM, utilization, power draw via `nvidia-smi`
- **Health evaluation**: Automatic WARN/CRIT status based on vLLM SRE thresholds
- **Delta/rate computation**: Tracks rps, token throughput, error rates between requests
- **Cross-platform**: Builds for x86_64 and ARM64 (NVIDIA DGX Spark)
- **Zero dependencies**: Single Go binary, just copy and run

## Quick Start

```bash
cd go/
go build -o vllm-monitor .

# First run: auto-detects services, generates config.yaml, starts server
./vllm-monitor

# Force re-detect
./vllm-monitor --detect

# Custom config
./vllm-monitor --config /path/to/config.yaml
```

## API Endpoints

### `GET /status`

Returns all services metrics and GPU overview.

```bash
curl http://localhost:9100/status
```

```json
{
  "timestamp": "2026-03-28T07:04:03Z",
  "gpus": [
    {
      "index": 0,
      "name": "NVIDIA RTX PRO 6000 Blackwell Workstation Edition",
      "temperature_c": 42,
      "utilization_pct": 0,
      "memory_used_mib": 86789,
      "memory_total_mib": 97887,
      "power_draw_w": 18.29
    }
  ],
  "services": [
    {
      "name": "vllm-gpt-oss-120b",
      "type": "vllm",
      "url": "http://127.0.0.1:8000",
      "reachable": true,
      "health": { "status": "OK", "reasons": [] },
      "metrics": {
        "traffic": { "rps": 12.5, "prompt_tps": 340.2, "gen_tps": 180.7 },
        "latency": { "avg_ttft_s": 0.222, "avg_e2e_s": 4.544 },
        "saturation": { "running": 3, "waiting": 0, "kv_cache_usage": 0.45, "preemptions": 0 },
        "errors": { "http_4xx_delta": 0, "http_5xx_delta": 0, "error_rate_pct": 0 },
        "efficiency": { "prefix_cache_hit_rate_pct": 44.64 }
      }
    },
    {
      "name": "ollama",
      "type": "ollama",
      "reachable": true,
      "health": { "status": "OK", "reasons": [] },
      "metrics": {
        "version": "0.13.1",
        "loaded_models": [],
        "available_models": ["bge-m3:latest"]
      }
    }
  ]
}
```

### `GET /gpu`

Returns detailed GPU card summary.

```bash
curl http://localhost:9100/gpu
```

```json
{
  "timestamp": "2026-03-28T07:04:10Z",
  "count": 1,
  "gpus": [
    {
      "index": 0,
      "uuid": "GPU-0cb7019c-...",
      "name": "NVIDIA RTX PRO 6000 Blackwell Workstation Edition",
      "driver_version": "580.95.05",
      "pstate": "P8",
      "memory_total_mib": 97887,
      "memory_used_mib": 86789,
      "memory_free_mib": 10460,
      "temperature_c": 42,
      "utilization_gpu_pct": 0,
      "utilization_memory_pct": 0,
      "power_draw_w": 18.64,
      "power_limit_w": 600.0
    }
  ]
}
```

## Health Status Thresholds

vLLM services are evaluated automatically:

| Status | Condition |
|--------|-----------|
| **WARN** | Requests waiting > 0, TTFT > 1.5s, E2E > 6.0s, HTTP 4xx |
| **CRIT** | KV cache > 85%, preemptions > 0, HTTP 5xx or errors |

## Config

Auto-generated on first run. Can also be edited manually:

```yaml
port: 9100
services:
  - name: vllm-gpt-oss-120b
    type: vllm
    url: http://127.0.0.1:8000
  - name: vllm-bge-m3
    type: vllm
    url: http://127.0.0.1:8001
  - name: ollama
    type: ollama
    url: http://127.0.0.1:11434
```

## Cross-Compile for ARM64

```bash
GOOS=linux GOARCH=arm64 go build -o vllm-monitor-arm64 .
```

## Bash Dashboard

A standalone terminal dashboard is also included for quick monitoring of a single vLLM instance:

```bash
./watch_vllm.sh                                        # default: localhost:8000
INTERVAL=10 ./watch_vllm.sh http://10.0.0.5:8000/metrics  # custom
```

## Project Structure

```
go/                     # Go version (primary)
  main.go               # Entry point, flag parsing, auto-detect logic
  detect.go             # Process scanner (/proc/*/cmdline)
  collector_vllm.go     # Prometheus metrics parser + health evaluation
  collector_ollama.go   # Ollama API poller
  gpu.go                # nvidia-smi integration
  handler_status.go     # GET /status handler
  handler_gpu.go        # GET /gpu handler
  types.go              # JSON response structs
  config.go             # YAML config read/write

server.py               # Python/FastAPI version (legacy)
parsers/                # Python metric parsers
watch_vllm.sh           # Bash terminal dashboard
```
