package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
)

func collectOllama(svc ServiceConfig, client *http.Client) ServiceResult {
	base := strings.TrimRight(svc.URL, "/")

	type result struct {
		body []byte
		err  error
	}

	fetch := func(path string) <-chan result {
		ch := make(chan result, 1)
		go func() {
			resp, err := client.Get(base + path)
			if err != nil {
				ch <- result{err: err}
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			ch <- result{body: body, err: err}
		}()
		return ch
	}

	psCh := fetch("/api/ps")
	tagsCh := fetch("/api/tags")
	verCh := fetch("/api/version")

	psRes := <-psCh
	tagsRes := <-tagsCh
	verRes := <-verCh

	// If all three failed, unreachable
	if psRes.err != nil && tagsRes.err != nil && verRes.err != nil {
		return ServiceResult{
			Name: svc.Name, Type: "ollama", URL: svc.URL,
			Reachable: false,
			Health:    Health{Status: "CRIT", Reasons: []string{"unreachable"}},
			Metrics:   nil,
		}
	}

	metrics := OllamaMetrics{
		LoadedModels:    []OllamaLoadedModel{},
		AvailableModels: []string{},
	}

	// Version
	if verRes.err == nil {
		var v struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(verRes.body, &v) == nil {
			metrics.Version = v.Version
		}
	}

	// Running models
	if psRes.err == nil {
		var ps struct {
			Models []struct {
				Name      string `json:"name"`
				Size      int64  `json:"size"`
				SizeVRAM  int64  `json:"size_vram"`
				ExpiresAt string `json:"expires_at"`
			} `json:"models"`
		}
		if json.Unmarshal(psRes.body, &ps) == nil {
			for _, m := range ps.Models {
				metrics.LoadedModels = append(metrics.LoadedModels, OllamaLoadedModel{
					Name:      m.Name,
					SizeBytes: m.Size,
					VRAMBytes: m.SizeVRAM,
					ExpiresAt: m.ExpiresAt,
				})
			}
		}
	}

	// Available models
	if tagsRes.err == nil {
		var tags struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if json.Unmarshal(tagsRes.body, &tags) == nil {
			for _, m := range tags.Models {
				metrics.AvailableModels = append(metrics.AvailableModels, m.Name)
			}
		}
	}

	return ServiceResult{
		Name: svc.Name, Type: "ollama", URL: svc.URL,
		Reachable: true,
		Health:    Health{Status: "OK", Reasons: []string{}},
		Metrics:   metrics,
	}
}

// ensure sync is importable
var _ sync.Mutex
