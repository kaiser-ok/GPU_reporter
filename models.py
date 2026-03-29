from __future__ import annotations

from datetime import datetime
from typing import Literal, Optional

from pydantic import BaseModel


class Health(BaseModel):
    status: Literal["OK", "WARN", "CRIT"]
    reasons: list[str]


class VllmTraffic(BaseModel):
    rps: Optional[float] = None
    prompt_tps: Optional[float] = None
    gen_tps: Optional[float] = None


class VllmLatency(BaseModel):
    avg_ttft_s: Optional[float] = None
    avg_e2e_s: Optional[float] = None


class VllmSaturation(BaseModel):
    running: float = 0
    waiting: float = 0
    kv_cache_usage: float = 0
    preemptions: float = 0


class VllmErrors(BaseModel):
    http_4xx_delta: Optional[int] = None
    http_5xx_delta: Optional[int] = None
    error_rate_pct: Optional[float] = None
    finished_stop: float = 0
    finished_length: float = 0
    finished_abort: float = 0
    finished_error: float = 0
    length_stop_rate_pct: float = 0


class VllmEfficiency(BaseModel):
    prefix_cache_hit_rate_pct: float = 0
    prefix_cache_hits: float = 0
    prefix_cache_queries: float = 0
    prompt_tokens_total: float = 0
    generation_tokens_total: float = 0
    http_requests_total: float = 0


class VllmMetrics(BaseModel):
    traffic: VllmTraffic
    latency: VllmLatency
    saturation: VllmSaturation
    errors: VllmErrors
    efficiency: VllmEfficiency


class OllamaLoadedModel(BaseModel):
    name: str
    size_bytes: int = 0
    vram_bytes: int = 0
    expires_at: Optional[str] = None


class OllamaMetrics(BaseModel):
    version: Optional[str] = None
    loaded_models: list[OllamaLoadedModel] = []
    available_models: list[str] = []


class GpuInfo(BaseModel):
    index: int
    name: str
    temperature_c: int
    utilization_pct: int
    memory_used_mib: int
    memory_total_mib: int
    power_draw_w: float


class GpuSummary(BaseModel):
    index: int
    uuid: str
    name: str
    driver_version: str
    pstate: str
    memory_total_mib: int
    memory_used_mib: int
    memory_free_mib: int
    temperature_c: int
    utilization_gpu_pct: int
    utilization_memory_pct: int
    power_draw_w: float
    power_limit_w: float


class GpuSummaryResponse(BaseModel):
    timestamp: datetime
    count: int
    gpus: list[GpuSummary]


class ServiceResult(BaseModel):
    name: str
    type: str
    url: str
    reachable: bool
    health: Health
    metrics: Optional[VllmMetrics | OllamaMetrics] = None


class StatusResponse(BaseModel):
    timestamp: datetime
    gpus: list[GpuInfo] = []
    services: list[ServiceResult]
