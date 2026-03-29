package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func detectServices() []ServiceConfig {
	var services []ServiceConfig
	seen := make(map[string]bool) // dedup by type+port

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return services
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Skip non-numeric (non-PID) directories
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}

		cmdlineBytes, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		args := strings.Split(string(cmdlineBytes), "\x00")
		// Remove empty trailing element
		if len(args) > 0 && args[len(args)-1] == "" {
			args = args[:len(args)-1]
		}
		if len(args) == 0 {
			continue
		}

		if svc, ok := detectVllm(args); ok {
			key := svc.Type + ":" + svc.URL
			if !seen[key] {
				seen[key] = true
				services = append(services, svc)
			}
		}

		if svc, ok := detectOllama(args); ok {
			key := svc.Type + ":" + svc.URL
			if !seen[key] {
				seen[key] = true
				services = append(services, svc)
			}
		}
	}

	return services
}

func detectVllm(args []string) (ServiceConfig, bool) {
	// Find "vllm" binary and "serve" subcommand
	serveIdx := -1
	for i, a := range args {
		base := filepath.Base(a)
		if base == "vllm" && i+1 < len(args) && args[i+1] == "serve" {
			serveIdx = i + 1
			break
		}
	}
	if serveIdx == -1 {
		return ServiceConfig{}, false
	}

	// Extract model path (first arg after "serve" that doesn't start with -)
	model := "unknown"
	for _, a := range args[serveIdx+1:] {
		if !strings.HasPrefix(a, "-") {
			model = filepath.Base(a)
			break
		}
	}

	// Extract --port
	port := "8000"
	for i, a := range args {
		if a == "--port" && i+1 < len(args) {
			port = args[i+1]
			break
		}
	}

	return ServiceConfig{
		Name: fmt.Sprintf("vllm-%s", model),
		Type: "vllm",
		URL:  fmt.Sprintf("http://127.0.0.1:%s", port),
	}, true
}

func detectOllama(args []string) (ServiceConfig, bool) {
	for i, a := range args {
		base := filepath.Base(a)
		if base == "ollama" && i+1 < len(args) && args[i+1] == "serve" {
			port := "11434"
			return ServiceConfig{
				Name: "ollama",
				Type: "ollama",
				URL:  fmt.Sprintf("http://127.0.0.1:%s", port),
			}, true
		}
	}
	return ServiceConfig{}, false
}
