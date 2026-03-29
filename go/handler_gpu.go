package main

import (
	"encoding/json"
	"net/http"
	"time"
)

func handleGpu(w http.ResponseWriter, r *http.Request) {
	gpus := fetchGpuSummary()
	if gpus == nil {
		gpus = []GpuSummary{}
	}

	resp := GpuSummaryResponse{
		Timestamp: time.Now().UTC(),
		Count:     len(gpus),
		GPUs:      gpus,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
