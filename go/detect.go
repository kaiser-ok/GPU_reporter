package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// scanForVllm probes each port in the range by hitting /metrics and looking
// for the vllm: metric prefix. This finds vLLM instances running in Docker
// or otherwise invisible to /proc scanning.
func scanForVllm(portStart, portEnd int) []ServiceConfig {
	var services []ServiceConfig
	client := &http.Client{Timeout: 1 * time.Second}
	modelRe := regexp.MustCompile(`model_name="([^"]+)"`)

	for port := portStart; port <= portEnd; port++ {
		url := fmt.Sprintf("http://127.0.0.1:%d/metrics", port)
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			continue
		}

		text := string(body)
		if !strings.Contains(text, "vllm:") {
			continue
		}

		// Try to extract model name from metric labels
		model := "unknown"
		if m := modelRe.FindStringSubmatch(text); m != nil {
			model = filepath.Base(m[1])
		}

		services = append(services, ServiceConfig{
			Name: fmt.Sprintf("vllm-%s", model),
			Type: "vllm",
			URL:  fmt.Sprintf("http://127.0.0.1:%d", port),
		})
	}
	return services
}

// parsePortRange parses "START-END" into two ints. Single port "8000" is also accepted.
func parsePortRange(s string) (int, int, error) {
	parts := strings.SplitN(s, "-", 2)
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid port range start: %w", err)
	}
	if len(parts) == 1 {
		return start, start, nil
	}
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid port range end: %w", err)
	}
	if end < start {
		return 0, 0, fmt.Errorf("port range end (%d) < start (%d)", end, start)
	}
	return start, end, nil
}

// probeVllm checks if a URL serves vLLM metrics.
func probeVllm(url string, client *http.Client) bool {
	resp, err := client.Get(strings.TrimRight(url, "/") + "/metrics")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	return strings.Contains(string(body), "vllm:")
}

// probeOllama checks if a URL serves the Ollama API.
func probeOllama(url string, client *http.Client) bool {
	resp, err := client.Get(strings.TrimRight(url, "/") + "/api/version")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func detectServices() []ServiceConfig {
	var services []ServiceConfig
	seen := make(map[string]bool) // dedup by type+port
	client := &http.Client{Timeout: 1 * time.Second}

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
				// Verify the service is actually reachable on the host
				// (container processes show up in /proc but their ports
				// may not match host-mapped ports)
				if probeVllm(svc.URL, client) {
					services = append(services, svc)
				} else {
					fmt.Printf("  Skipping %s at %s (not reachable on host)\n", svc.Name, svc.URL)
				}
			}
		}

		if svc, ok := detectOllama(args); ok {
			key := svc.Type + ":" + svc.URL
			if !seen[key] {
				seen[key] = true
				if probeOllama(svc.URL, client) {
					services = append(services, svc)
				} else {
					fmt.Printf("  Skipping %s at %s (not reachable on host)\n", svc.Name, svc.URL)
				}
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
