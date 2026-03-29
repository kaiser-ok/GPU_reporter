# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Monitoring and API tooling for inference services running on this host:
- **vLLM GPT-OSS-120B** (port 8000) — LLM generation, systemd: `vllm.service`
- **vLLM BAAI/bge-m3** (port 8001) — embedding (pooling runner), systemd: `vllm-embedding.service`
- **Ollama** (port 11434) — CPU-only, systemd: `ollama.service`

Three components:
1. `watch_vllm.sh` — Terminal dashboard polling a single vLLM Prometheus endpoint
2. `server.py` — Python/FastAPI version of the API (legacy)
3. `go/` — **Go version (primary)** — single binary with auto-detect, same API

## Running (Go version — recommended)

```bash
cd go/

# First run: auto-detects vLLM/Ollama processes, generates config.yaml, starts server
./vllm-monitor

# Force re-detect services
./vllm-monitor --detect

# Custom config path
./vllm-monitor --config /etc/vllm-monitor.yaml

# Build from source
go build -o vllm-monitor .
```

## Running (Python version)

```bash
pip install -r requirements.txt
python3 server.py
CONFIG_PATH=/etc/myconfig.yaml python3 server.py
```

## Running (bash dashboard)

```bash
./watch_vllm.sh
INTERVAL=10 ./watch_vllm.sh http://127.0.0.1:8001/metrics
```

## API Endpoints

- `GET /status` — All services metrics + GPU overview (JSON)
- `GET /gpu` — Full GPU card summary (JSON)

## Architecture (Go)

```
go/
  main.go              — Entry: flag parsing, detect-or-load config, start HTTP server
  config.go            — config.yaml read/write
  detect.go            — Process scan: reads /proc/*/cmdline, finds vllm/ollama processes
  handler_status.go    — GET /status: goroutine fan-out to collectors
  handler_gpu.go       — GET /gpu: nvidia-smi query
  collector_vllm.go    — Prometheus text parser, delta computation, health evaluation
  collector_ollama.go  — Ollama /api/ps, /api/tags, /api/version poller
  gpu.go               — nvidia-smi exec + CSV parsing
  types.go             — All struct definitions (JSON response models)
```

## Architecture (Python — legacy)

```
server.py            — FastAPI app, config loading, GET /status, async fan-out
models.py            — Pydantic response schemas
parsers/vllm.py      — Prometheus text parser, delta/rate computation, health eval
parsers/ollama.py    — Ollama API poller
parsers/gpu.py       — nvidia-smi exec
config.yaml          — Service registry
```

## Auto-Detection

Go version scans `/proc/[pid]/cmdline` for:
- **vLLM**: processes containing `vllm serve`, extracts `--port` (default 8000) and model name from path
- **Ollama**: processes containing `ollama serve`, uses default port 11434

Detection runs automatically when `config.yaml` doesn't exist, or with `--detect` flag.

## Key Metrics (vLLM)

Scraped from Prometheus `/metrics` endpoint. Metric names use `vllm:` prefix (e.g., `vllm:num_requests_running`, `vllm:kv_cache_usage_perc`). HTTP metrics use `http_requests_total{status="..."}`.

- **Traffic**: rps, prompt tokens/sec, generation tokens/sec
- **Latency**: avg TTFT, avg e2e
- **Saturation**: running/waiting requests, KV cache %, preemptions
- **Errors**: HTTP 4xx/5xx deltas, finish reasons, error rate
- **Efficiency**: prefix cache hit rate, cumulative token counts

## Health Status Thresholds

Implemented in `watch_vllm.sh`, `parsers/vllm.py`, and `go/collector_vllm.go` — keep in sync.
- **WARN**: waiting > 0, TTFT > 1.5s, e2e > 6.0s, client 4xx
- **CRIT**: KV cache > 85%, preemptions > 0, 5xx/errors
