from __future__ import annotations

import asyncio

from models import GpuInfo, GpuSummary


async def fetch_gpu_info() -> list[GpuInfo]:
    """Query nvidia-smi for GPU stats."""
    proc = await asyncio.create_subprocess_exec(
        "nvidia-smi",
        "--query-gpu=index,name,temperature.gpu,utilization.gpu,memory.used,memory.total,power.draw",
        "--format=csv,noheader,nounits",
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    stdout, _ = await proc.communicate()
    if proc.returncode != 0:
        return []

    gpus: list[GpuInfo] = []
    for line in stdout.decode().strip().splitlines():
        parts = [p.strip() for p in line.split(",")]
        if len(parts) < 7:
            continue
        gpus.append(GpuInfo(
            index=int(parts[0]),
            name=parts[1],
            temperature_c=int(parts[2]),
            utilization_pct=int(parts[3]),
            memory_used_mib=int(parts[4]),
            memory_total_mib=int(parts[5]),
            power_draw_w=float(parts[6]),
        ))
    return gpus


async def fetch_gpu_summary() -> list[GpuSummary]:
    """Query nvidia-smi for full GPU card summary."""
    proc = await asyncio.create_subprocess_exec(
        "nvidia-smi",
        "--query-gpu=index,name,driver_version,memory.total,memory.used,memory.free,"
        "temperature.gpu,utilization.gpu,utilization.memory,power.draw,power.limit,pstate,gpu_uuid",
        "--format=csv,noheader,nounits",
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    stdout, _ = await proc.communicate()
    if proc.returncode != 0:
        return []

    gpus: list[GpuSummary] = []
    for line in stdout.decode().strip().splitlines():
        parts = [p.strip() for p in line.split(",")]
        if len(parts) < 13:
            continue
        gpus.append(GpuSummary(
            index=int(parts[0]),
            name=parts[1],
            driver_version=parts[2],
            memory_total_mib=int(parts[3]),
            memory_used_mib=int(parts[4]),
            memory_free_mib=int(parts[5]),
            temperature_c=int(parts[6]),
            utilization_gpu_pct=int(parts[7]),
            utilization_memory_pct=int(parts[8]),
            power_draw_w=float(parts[9]),
            power_limit_w=float(parts[10]),
            pstate=parts[11],
            uuid=parts[12],
        ))
    return gpus
