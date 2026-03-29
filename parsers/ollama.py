from __future__ import annotations

import httpx

from models import Health, OllamaLoadedModel, OllamaMetrics


async def fetch_ollama(client: httpx.AsyncClient, base_url: str) -> OllamaMetrics:
    """Poll Ollama API endpoints and return an OllamaMetrics snapshot."""
    base = base_url.rstrip("/")

    # Fetch all three endpoints concurrently
    import asyncio
    ps_resp, tags_resp, ver_resp = await asyncio.gather(
        client.get(f"{base}/api/ps"),
        client.get(f"{base}/api/tags"),
        client.get(f"{base}/api/version"),
        return_exceptions=True,
    )

    # Version
    version = None
    if isinstance(ver_resp, httpx.Response) and ver_resp.status_code == 200:
        version = ver_resp.json().get("version")

    # Running models
    loaded_models: list[OllamaLoadedModel] = []
    if isinstance(ps_resp, httpx.Response) and ps_resp.status_code == 200:
        for m in ps_resp.json().get("models", []):
            loaded_models.append(OllamaLoadedModel(
                name=m.get("name", ""),
                size_bytes=m.get("size", 0),
                vram_bytes=m.get("size_vram", 0),
                expires_at=m.get("expires_at"),
            ))

    # Available models
    available_models: list[str] = []
    if isinstance(tags_resp, httpx.Response) and tags_resp.status_code == 200:
        for m in tags_resp.json().get("models", []):
            available_models.append(m.get("name", ""))

    return OllamaMetrics(
        version=version,
        loaded_models=loaded_models,
        available_models=available_models,
    )


def evaluate_health(reachable: bool) -> Health:
    """Ollama health: CRIT if unreachable, OK otherwise."""
    if not reachable:
        return Health(status="CRIT", reasons=["unreachable"])
    return Health(status="OK", reasons=[])
