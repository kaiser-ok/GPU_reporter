from __future__ import annotations

import asyncio
import os
from contextlib import asynccontextmanager
from datetime import datetime, timezone

import httpx
import yaml
from fastapi import FastAPI

from models import GpuSummaryResponse, Health, ServiceResult, StatusResponse
from parsers import gpu as gpu_parser
from parsers import ollama as ollama_parser
from parsers import vllm as vllm_parser

CONFIG_PATH = os.environ.get("CONFIG_PATH", "./config.yaml")

config: dict = {}
http_client: httpx.AsyncClient


def load_config(path: str) -> dict:
    with open(path) as f:
        return yaml.safe_load(f)


@asynccontextmanager
async def lifespan(app: FastAPI):
    global config, http_client
    config = load_config(CONFIG_PATH)
    http_client = httpx.AsyncClient(timeout=3.0)
    yield
    await http_client.aclose()


app = FastAPI(title="Inference Metrics API", lifespan=lifespan)


async def collect_vllm(service: dict) -> ServiceResult:
    name = service["name"]
    url = service["url"].rstrip("/")
    try:
        resp = await http_client.get(f"{url}/metrics")
        resp.raise_for_status()
        metrics = vllm_parser.extract_snapshot(name, resp.text)
        health = vllm_parser.evaluate_health(metrics)
        return ServiceResult(
            name=name, type="vllm", url=service["url"],
            reachable=True, health=health, metrics=metrics,
        )
    except Exception:
        return ServiceResult(
            name=name, type="vllm", url=service["url"],
            reachable=False,
            health=Health(status="CRIT", reasons=["unreachable"]),
            metrics=None,
        )


async def collect_ollama(service: dict) -> ServiceResult:
    name = service["name"]
    try:
        metrics = await ollama_parser.fetch_ollama(http_client, service["url"])
        health = ollama_parser.evaluate_health(reachable=True)
        return ServiceResult(
            name=name, type="ollama", url=service["url"],
            reachable=True, health=health, metrics=metrics,
        )
    except Exception:
        return ServiceResult(
            name=name, type="ollama", url=service["url"],
            reachable=False,
            health=ollama_parser.evaluate_health(reachable=False),
            metrics=None,
        )


COLLECTORS = {
    "vllm": collect_vllm,
    "ollama": collect_ollama,
}


@app.get("/status", response_model=StatusResponse)
async def get_status():
    tasks = []
    for svc in config.get("services", []):
        collector = COLLECTORS.get(svc["type"])
        if collector:
            tasks.append(collector(svc))
    results = await asyncio.gather(*tasks, return_exceptions=True)

    services = []
    for i, result in enumerate(results):
        if isinstance(result, Exception):
            svc = config["services"][i]
            services.append(ServiceResult(
                name=svc["name"], type=svc["type"], url=svc["url"],
                reachable=False,
                health=Health(status="CRIT", reasons=["collector_error"]),
                metrics=None,
            ))
        else:
            services.append(result)

    gpus = await gpu_parser.fetch_gpu_info()

    return StatusResponse(
        timestamp=datetime.now(timezone.utc),
        gpus=gpus,
        services=services,
    )


@app.get("/gpu", response_model=GpuSummaryResponse)
async def get_gpu():
    gpus = await gpu_parser.fetch_gpu_summary()
    return GpuSummaryResponse(
        timestamp=datetime.now(timezone.utc),
        count=len(gpus),
        gpus=gpus,
    )


if __name__ == "__main__":
    import uvicorn
    cfg = load_config(CONFIG_PATH)
    uvicorn.run(app, host="0.0.0.0", port=cfg.get("port", 9100))
