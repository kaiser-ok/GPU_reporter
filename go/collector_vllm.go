package main

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type vllmCounters struct {
	PromptTokens float64
	GenTokens    float64
	HTTPTotal    float64
	HTTP4xx      float64
	HTTP5xx      float64
	Stop         float64
	Length       float64
	Abort        float64
	Error        float64
}

type prevState struct {
	counters vllmCounters
	ts       float64
}

var (
	prevMu    sync.Mutex
	prevStore = make(map[string]prevState)
)

var metricLineRe = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*(?:\{[^}]*\})?)\s+([\d.eE+\-]+)`)

func parsePrometheusText(text string) map[string]float64 {
	result := make(map[string]float64)
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		m := metricLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		v, err := strconv.ParseFloat(m[2], 64)
		if err == nil {
			result[m[1]] = v
		}
	}
	return result
}

func getMetric(parsed map[string]float64, pattern string) float64 {
	re := regexp.MustCompile(pattern)
	for k, v := range parsed {
		if re.MatchString(k) {
			return v
		}
	}
	return 0
}

func sumMetrics(parsed map[string]float64, pattern string) float64 {
	re := regexp.MustCompile(pattern)
	total := 0.0
	for k, v := range parsed {
		if re.MatchString(k) {
			total += v
		}
	}
	return total
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func roundN(v float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.Round(v*p) / p
}

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }

func collectVllm(svc ServiceConfig, client *http.Client) ServiceResult {
	url := strings.TrimRight(svc.URL, "/") + "/metrics"
	resp, err := client.Get(url)
	if err != nil {
		return unreachableResult(svc, "vllm")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return unreachableResult(svc, "vllm")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return unreachableResult(svc, "vllm")
	}

	parsed := parsePrometheusText(string(body))
	now := float64(time.Now().UnixMilli()) / 1000.0

	// Core state
	running := getMetric(parsed, `^vllm:num_requests_running\{`)
	waiting := getMetric(parsed, `^vllm:num_requests_waiting\{`)
	kvUsage := getMetric(parsed, `^vllm:kv_cache_usage_perc\{`)
	preemptions := getMetric(parsed, `^vllm:num_preemptions_total\{`)

	// Latency
	ttftCount := getMetric(parsed, `^vllm:time_to_first_token_seconds_count\{`)
	ttftSum := getMetric(parsed, `^vllm:time_to_first_token_seconds_sum\{`)
	e2eCount := getMetric(parsed, `^vllm:e2e_request_latency_seconds_count\{`)
	e2eSum := getMetric(parsed, `^vllm:e2e_request_latency_seconds_sum\{`)
	avgTTFT := safeDiv(ttftSum, ttftCount)
	avgE2E := safeDiv(e2eSum, e2eCount)

	// Counters
	counters := vllmCounters{
		PromptTokens: getMetric(parsed, `^vllm:prompt_tokens_total\{`),
		GenTokens:    getMetric(parsed, `^vllm:generation_tokens_total\{`),
		HTTPTotal:    sumMetrics(parsed, `^http_requests_total\{`),
		HTTP4xx:      sumMetrics(parsed, `^http_requests_total\{.*status="4xx"`),
		HTTP5xx:      sumMetrics(parsed, `^http_requests_total\{.*status="5xx"`),
		Stop:         getMetric(parsed, `^vllm:request_success_total\{.*finished_reason="stop"`),
		Length:       getMetric(parsed, `^vllm:request_success_total\{.*finished_reason="length"`),
		Abort:        getMetric(parsed, `^vllm:request_success_total\{.*finished_reason="abort"`),
		Error:        getMetric(parsed, `^vllm:request_success_total\{.*finished_reason="error"`),
	}

	// Prefix cache
	prefixQ := getMetric(parsed, `^vllm:prefix_cache_queries_total\{`)
	prefixH := getMetric(parsed, `^vllm:prefix_cache_hits_total\{`)
	prefixHitPct := roundN(safeDiv(prefixH, prefixQ)*100, 2)

	// Finish reason rate
	totalFinished := counters.Stop + counters.Length + counters.Abort + counters.Error
	lengthStopPct := roundN(safeDiv(counters.Length, totalFinished)*100, 2)

	// Delta computation
	var rps, promptTPS, genTPS *float64
	var d4xx, d5xx *int
	var errRatePct *float64

	prevMu.Lock()
	prev, hasPrev := prevStore[svc.Name]
	prevStore[svc.Name] = prevState{counters: counters, ts: now}
	prevMu.Unlock()

	if hasPrev {
		elapsed := now - prev.ts
		if elapsed <= 0 {
			elapsed = 1
		}
		promptTPS = fp((counters.PromptTokens - prev.counters.PromptTokens) / elapsed)
		genTPS = fp((counters.GenTokens - prev.counters.GenTokens) / elapsed)
		rps = fp((counters.HTTPTotal - prev.counters.HTTPTotal) / elapsed)
		d4xx = ip(int(counters.HTTP4xx - prev.counters.HTTP4xx))
		d5xx = ip(int(counters.HTTP5xx - prev.counters.HTTP5xx))
		dStop := counters.Stop - prev.counters.Stop
		dLength := counters.Length - prev.counters.Length
		dAbort := counters.Abort - prev.counters.Abort
		dError := counters.Error - prev.counters.Error
		intervalTotal := dStop + dLength + dAbort + dError + float64(*d4xx) + float64(*d5xx)
		errRatePct = fp(roundN(safeDiv(dError+float64(*d5xx), intervalTotal)*100, 2))
	}

	metrics := VllmMetrics{
		Traffic: VllmTraffic{RPS: rps, PromptTPS: promptTPS, GenTPS: genTPS},
		Latency: VllmLatency{
			AvgTTFT: fp(roundN(avgTTFT, 6)),
			AvgE2E:  fp(roundN(avgE2E, 6)),
		},
		Saturation: VllmSaturation{
			Running: running, Waiting: waiting,
			KVCache: kvUsage, Preemption: preemptions,
		},
		Errors: VllmErrors{
			HTTP4xxDelta: d4xx, HTTP5xxDelta: d5xx,
			ErrorRatePct:      errRatePct,
			FinishedStop:      counters.Stop,
			FinishedLength:    counters.Length,
			FinishedAbort:     counters.Abort,
			FinishedError:     counters.Error,
			LengthStopRatePct: lengthStopPct,
		},
		Efficiency: VllmEfficiency{
			PrefixCacheHitRatePct: prefixHitPct,
			PrefixCacheHits:       prefixH,
			PrefixCacheQueries:    prefixQ,
			PromptTokensTotal:     counters.PromptTokens,
			GenTokensTotal:        counters.GenTokens,
			HTTPRequestsTotal:     counters.HTTPTotal,
		},
	}

	health := evaluateVllmHealth(metrics)

	return ServiceResult{
		Name: svc.Name, Type: "vllm", URL: svc.URL,
		Reachable: true, Health: health, Metrics: metrics,
	}
}

func evaluateVllmHealth(m VllmMetrics) Health {
	status := "OK"
	var reasons []string

	if m.Saturation.Waiting > 0 {
		status = "WARN"
		reasons = append(reasons, "queue")
	}
	if m.Latency.AvgTTFT != nil && *m.Latency.AvgTTFT > 1.5 {
		status = "WARN"
		reasons = append(reasons, "slow_ttft")
	}
	if m.Latency.AvgE2E != nil && *m.Latency.AvgE2E > 6.0 {
		status = "WARN"
		reasons = append(reasons, "slow_e2e")
	}

	if m.Saturation.KVCache > 0.85 {
		status = "CRIT"
		reasons = append(reasons, "kv_pressure")
	}
	if m.Saturation.Preemption > 0 {
		status = "CRIT"
		reasons = append(reasons, "preemption")
	}
	if (m.Errors.HTTP5xxDelta != nil && *m.Errors.HTTP5xxDelta > 0) ||
		(m.Errors.FinishedError > 0 && m.Errors.HTTP5xxDelta != nil) {
		status = "CRIT"
		reasons = append(reasons, "errors")
	}

	if m.Errors.HTTP4xxDelta != nil && *m.Errors.HTTP4xxDelta > 0 {
		if status == "OK" {
			status = "WARN"
		}
		reasons = append(reasons, "client_4xx")
	}

	if reasons == nil {
		reasons = []string{}
	}
	return Health{Status: status, Reasons: reasons}
}

func unreachableResult(svc ServiceConfig, svcType string) ServiceResult {
	return ServiceResult{
		Name: svc.Name, Type: svcType, URL: svc.URL,
		Reachable: false,
		Health:    Health{Status: "CRIT", Reasons: []string{"unreachable"}},
		Metrics:   nil,
	}
}

func init() {
	_ = fmt.Sprintf // ensure fmt is used
}
