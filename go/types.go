package main

import "time"

// --- Health ---

type Health struct {
	Status  string   `json:"status"`
	Reasons []string `json:"reasons"`
}

// --- vLLM metrics ---

type VllmTraffic struct {
	RPS       *float64 `json:"rps"`
	PromptTPS *float64 `json:"prompt_tps"`
	GenTPS    *float64 `json:"gen_tps"`
}

type VllmLatency struct {
	AvgTTFT *float64 `json:"avg_ttft_s"`
	AvgE2E  *float64 `json:"avg_e2e_s"`
}

type VllmSaturation struct {
	Running    float64 `json:"running"`
	Waiting    float64 `json:"waiting"`
	KVCache    float64 `json:"kv_cache_usage"`
	Preemption float64 `json:"preemptions"`
}

type VllmErrors struct {
	HTTP4xxDelta      *int     `json:"http_4xx_delta"`
	HTTP5xxDelta      *int     `json:"http_5xx_delta"`
	ErrorRatePct      *float64 `json:"error_rate_pct"`
	FinishedStop      float64  `json:"finished_stop"`
	FinishedLength    float64  `json:"finished_length"`
	FinishedAbort     float64  `json:"finished_abort"`
	FinishedError     float64  `json:"finished_error"`
	LengthStopRatePct float64  `json:"length_stop_rate_pct"`
}

type VllmEfficiency struct {
	PrefixCacheHitRatePct float64 `json:"prefix_cache_hit_rate_pct"`
	PrefixCacheHits       float64 `json:"prefix_cache_hits"`
	PrefixCacheQueries    float64 `json:"prefix_cache_queries"`
	PromptTokensTotal     float64 `json:"prompt_tokens_total"`
	GenTokensTotal        float64 `json:"generation_tokens_total"`
	HTTPRequestsTotal     float64 `json:"http_requests_total"`
}

type VllmMetrics struct {
	Traffic    VllmTraffic    `json:"traffic"`
	Latency    VllmLatency    `json:"latency"`
	Saturation VllmSaturation `json:"saturation"`
	Errors     VllmErrors     `json:"errors"`
	Efficiency VllmEfficiency `json:"efficiency"`
}

// --- Ollama metrics ---

type OllamaLoadedModel struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	VRAMBytes int64  `json:"vram_bytes"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type OllamaMetrics struct {
	Version         string              `json:"version,omitempty"`
	LoadedModels    []OllamaLoadedModel `json:"loaded_models"`
	AvailableModels []string            `json:"available_models"`
}

// --- GPU ---

type GpuInfo struct {
	Index         int     `json:"index"`
	Name          string  `json:"name"`
	TemperatureC  int     `json:"temperature_c"`
	UtilPct       int     `json:"utilization_pct"`
	MemUsedMiB    int     `json:"memory_used_mib"`
	MemTotalMiB   int     `json:"memory_total_mib"`
	PowerDrawW    float64 `json:"power_draw_w"`
}

type GpuSummary struct {
	Index         int     `json:"index"`
	UUID          string  `json:"uuid"`
	Name          string  `json:"name"`
	DriverVersion string  `json:"driver_version"`
	PState        string  `json:"pstate"`
	MemTotalMiB   int     `json:"memory_total_mib"`
	MemUsedMiB    int     `json:"memory_used_mib"`
	MemFreeMiB    int     `json:"memory_free_mib"`
	TemperatureC  int     `json:"temperature_c"`
	UtilGPUPct    int     `json:"utilization_gpu_pct"`
	UtilMemPct    int     `json:"utilization_memory_pct"`
	PowerDrawW    float64 `json:"power_draw_w"`
	PowerLimitW   float64 `json:"power_limit_w"`
}

// --- API responses ---

type ServiceResult struct {
	Name      string      `json:"name"`
	Type      string      `json:"type"`
	URL       string      `json:"url"`
	Reachable bool        `json:"reachable"`
	Health    Health      `json:"health"`
	Metrics   interface{} `json:"metrics"`
}

type StatusResponse struct {
	Timestamp time.Time       `json:"timestamp"`
	GPUs      []GpuInfo       `json:"gpus"`
	Services  []ServiceResult `json:"services"`
}

type GpuSummaryResponse struct {
	Timestamp time.Time    `json:"timestamp"`
	Count     int          `json:"count"`
	GPUs      []GpuSummary `json:"gpus"`
}
