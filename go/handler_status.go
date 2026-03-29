package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

func handleStatus(cfg *Config, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var wg sync.WaitGroup
		results := make([]ServiceResult, len(cfg.Services))

		for i, svc := range cfg.Services {
			wg.Add(1)
			go func(idx int, s ServiceConfig) {
				defer wg.Done()
				switch s.Type {
				case "vllm":
					results[idx] = collectVllm(s, client)
				case "ollama":
					results[idx] = collectOllama(s, client)
				default:
					results[idx] = unreachableResult(s, s.Type)
				}
			}(i, svc)
		}
		wg.Wait()

		gpus := fetchGpuInfo()
		if gpus == nil {
			gpus = []GpuInfo{}
		}

		resp := StatusResponse{
			Timestamp: time.Now().UTC(),
			GPUs:      gpus,
			Services:  results,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
