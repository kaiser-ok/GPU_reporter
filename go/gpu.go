package main

import (
	"os/exec"
	"strconv"
	"strings"
)

func fetchGpuInfo() []GpuInfo {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=index,name,temperature.gpu,utilization.gpu,memory.used,memory.total,power.draw",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return []GpuInfo{}
	}

	var gpus []GpuInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := splitCSV(line)
		if len(parts) < 7 {
			continue
		}
		gpus = append(gpus, GpuInfo{
			Index:        atoi(parts[0]),
			Name:         parts[1],
			TemperatureC: atoi(parts[2]),
			UtilPct:      atoi(parts[3]),
			MemUsedMiB:   atoi(parts[4]),
			MemTotalMiB:  atoi(parts[5]),
			PowerDrawW:   atof(parts[6]),
		})
	}
	return gpus
}

func fetchGpuSummary() []GpuSummary {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=index,name,driver_version,memory.total,memory.used,memory.free,"+
			"temperature.gpu,utilization.gpu,utilization.memory,power.draw,power.limit,pstate,gpu_uuid",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return []GpuSummary{}
	}

	var gpus []GpuSummary
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := splitCSV(line)
		if len(parts) < 13 {
			continue
		}
		gpus = append(gpus, GpuSummary{
			Index:         atoi(parts[0]),
			Name:          parts[1],
			DriverVersion: parts[2],
			MemTotalMiB:   atoi(parts[3]),
			MemUsedMiB:    atoi(parts[4]),
			MemFreeMiB:    atoi(parts[5]),
			TemperatureC:  atoi(parts[6]),
			UtilGPUPct:    atoi(parts[7]),
			UtilMemPct:    atoi(parts[8]),
			PowerDrawW:    atof(parts[9]),
			PowerLimitW:   atof(parts[10]),
			PState:        parts[11],
			UUID:          parts[12],
		})
	}
	return gpus
}

func splitCSV(line string) []string {
	parts := strings.Split(line, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func atoi(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

func atof(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}
