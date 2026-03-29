from __future__ import annotations

import re
import time
from dataclasses import dataclass, field
from typing import Optional

from models import (
    Health,
    VllmEfficiency,
    VllmErrors,
    VllmLatency,
    VllmMetrics,
    VllmSaturation,
    VllmTraffic,
)

# In-memory state for delta computation: {service_name: (counters, timestamp)}
_previous: dict[str, tuple[_Counters, float]] = {}


@dataclass
class _Counters:
    """Raw cumulative counters extracted from Prometheus text."""
    prompt_tokens_total: float = 0
    generation_tokens_total: float = 0
    http_total: float = 0
    http_4xx: float = 0
    http_5xx: float = 0
    finished_stop: float = 0
    finished_length: float = 0
    finished_abort: float = 0
    finished_error: float = 0


def parse_prometheus_text(text: str) -> dict[str, float]:
    """Parse Prometheus exposition format into {metric_key: value}."""
    result: dict[str, float] = {}
    for match in re.finditer(
        r'^([a-zA-Z_:][a-zA-Z0-9_:]*(?:\{[^}]*\})?)\s+([\d.eE+\-]+)',
        text,
        re.MULTILINE,
    ):
        try:
            result[match.group(1)] = float(match.group(2))
        except ValueError:
            continue
    return result


def _get_metric(parsed: dict[str, float], pattern: str) -> float:
    """Return first value whose key matches regex pattern, or 0."""
    pat = re.compile(pattern)
    for key, val in parsed.items():
        if pat.search(key):
            return val
    return 0


def _sum_metrics(parsed: dict[str, float], pattern: str) -> float:
    """Sum all values whose keys match regex pattern."""
    pat = re.compile(pattern)
    total = 0.0
    for key, val in parsed.items():
        if pat.search(key):
            total += val
    return total


def _safe_div(a: float, b: float) -> float:
    return a / b if b != 0 else 0


def extract_snapshot(service_name: str, raw_text: str) -> VllmMetrics:
    """Parse raw Prometheus text and return a VllmMetrics snapshot with rates."""
    parsed = parse_prometheus_text(raw_text)
    now = time.time()

    # Core state
    running = _get_metric(parsed, r'^vllm:num_requests_running\{')
    waiting = _get_metric(parsed, r'^vllm:num_requests_waiting\{')
    kv_usage = _get_metric(parsed, r'^vllm:kv_cache_usage_perc\{')
    preemptions = _get_metric(parsed, r'^vllm:num_preemptions_total\{')

    # Latency
    ttft_count = _get_metric(parsed, r'^vllm:time_to_first_token_seconds_count\{')
    ttft_sum = _get_metric(parsed, r'^vllm:time_to_first_token_seconds_sum\{')
    e2e_count = _get_metric(parsed, r'^vllm:e2e_request_latency_seconds_count\{')
    e2e_sum = _get_metric(parsed, r'^vllm:e2e_request_latency_seconds_sum\{')
    avg_ttft = _safe_div(ttft_sum, ttft_count)
    avg_e2e = _safe_div(e2e_sum, e2e_count)

    # Current cumulative counters
    counters = _Counters(
        prompt_tokens_total=_get_metric(parsed, r'^vllm:prompt_tokens_total\{'),
        generation_tokens_total=_get_metric(parsed, r'^vllm:generation_tokens_total\{'),
        http_total=_sum_metrics(parsed, r'^http_requests_total\{'),
        http_4xx=_sum_metrics(parsed, r'^http_requests_total\{.*status="4xx"'),
        http_5xx=_sum_metrics(parsed, r'^http_requests_total\{.*status="5xx"'),
        finished_stop=_get_metric(parsed, r'^vllm:request_success_total\{.*finished_reason="stop"'),
        finished_length=_get_metric(parsed, r'^vllm:request_success_total\{.*finished_reason="length"'),
        finished_abort=_get_metric(parsed, r'^vllm:request_success_total\{.*finished_reason="abort"'),
        finished_error=_get_metric(parsed, r'^vllm:request_success_total\{.*finished_reason="error"'),
    )

    # Prefix cache
    prefix_queries = _get_metric(parsed, r'^vllm:prefix_cache_queries_total\{')
    prefix_hits = _get_metric(parsed, r'^vllm:prefix_cache_hits_total\{')
    prefix_hit_rate_pct = _safe_div(prefix_hits, prefix_queries) * 100

    # Finish reason rates
    total_finished = (
        counters.finished_stop + counters.finished_length
        + counters.finished_abort + counters.finished_error
    )
    length_stop_rate_pct = _safe_div(counters.finished_length, total_finished) * 100

    # Delta / rate computation
    rps: Optional[float] = None
    prompt_tps: Optional[float] = None
    gen_tps: Optional[float] = None
    d_4xx: Optional[int] = None
    d_5xx: Optional[int] = None
    error_rate_pct: Optional[float] = None

    prev = _previous.get(service_name)
    if prev is not None:
        prev_counters, prev_ts = prev
        elapsed = now - prev_ts
        if elapsed <= 0:
            elapsed = 1.0

        prompt_tps = (counters.prompt_tokens_total - prev_counters.prompt_tokens_total) / elapsed
        gen_tps = (counters.generation_tokens_total - prev_counters.generation_tokens_total) / elapsed
        rps = (counters.http_total - prev_counters.http_total) / elapsed

        d_4xx = int(counters.http_4xx - prev_counters.http_4xx)
        d_5xx = int(counters.http_5xx - prev_counters.http_5xx)
        d_stop = counters.finished_stop - prev_counters.finished_stop
        d_length = counters.finished_length - prev_counters.finished_length
        d_abort = counters.finished_abort - prev_counters.finished_abort
        d_error = counters.finished_error - prev_counters.finished_error
        interval_total = d_stop + d_length + d_abort + d_error + d_4xx + d_5xx
        error_rate_pct = _safe_div(d_error + d_5xx, interval_total) * 100

    # Store current counters for next call
    _previous[service_name] = (counters, now)

    return VllmMetrics(
        traffic=VllmTraffic(rps=rps, prompt_tps=prompt_tps, gen_tps=gen_tps),
        latency=VllmLatency(avg_ttft_s=round(avg_ttft, 6), avg_e2e_s=round(avg_e2e, 6)),
        saturation=VllmSaturation(
            running=running, waiting=waiting,
            kv_cache_usage=kv_usage, preemptions=preemptions,
        ),
        errors=VllmErrors(
            http_4xx_delta=d_4xx, http_5xx_delta=d_5xx,
            error_rate_pct=error_rate_pct,
            finished_stop=counters.finished_stop,
            finished_length=counters.finished_length,
            finished_abort=counters.finished_abort,
            finished_error=counters.finished_error,
            length_stop_rate_pct=round(length_stop_rate_pct, 2),
        ),
        efficiency=VllmEfficiency(
            prefix_cache_hit_rate_pct=round(prefix_hit_rate_pct, 2),
            prefix_cache_hits=prefix_hits,
            prefix_cache_queries=prefix_queries,
            prompt_tokens_total=counters.prompt_tokens_total,
            generation_tokens_total=counters.generation_tokens_total,
            http_requests_total=counters.http_total,
        ),
    )


def evaluate_health(metrics: VllmMetrics) -> Health:
    """Evaluate health status using the same thresholds as watch_vllm.sh."""
    status = "OK"
    reasons: list[str] = []

    # WARN conditions
    if metrics.saturation.waiting > 0:
        status = "WARN"
        reasons.append("queue")
    if metrics.latency.avg_ttft_s is not None and metrics.latency.avg_ttft_s > 1.5:
        status = "WARN"
        reasons.append("slow_ttft")
    if metrics.latency.avg_e2e_s is not None and metrics.latency.avg_e2e_s > 6.0:
        status = "WARN"
        reasons.append("slow_e2e")

    # CRIT conditions (override WARN)
    if metrics.saturation.kv_cache_usage > 0.85:
        status = "CRIT"
        reasons.append("kv_pressure")
    if metrics.saturation.preemptions > 0:
        status = "CRIT"
        reasons.append("preemption")
    if (metrics.errors.http_5xx_delta is not None and metrics.errors.http_5xx_delta > 0) or \
       (metrics.errors.finished_error > 0 and metrics.errors.http_5xx_delta is not None):
        status = "CRIT"
        reasons.append("errors")

    # 4xx only bumps to WARN if still OK
    if metrics.errors.http_4xx_delta is not None and metrics.errors.http_4xx_delta > 0:
        if status == "OK":
            status = "WARN"
        reasons.append("client_4xx")

    return Health(status=status, reasons=reasons)
