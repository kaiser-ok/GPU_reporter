package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const defaultRescanInterval = 30 * time.Minute

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	detect := flag.Bool("detect", false, "force re-detect services and regenerate config")
	scanPorts := flag.String("scan-ports", "", "port range to scan for vLLM (e.g. 8000-8010); overrides config for this run")
	flag.Parse()

	// Auto-detect if config doesn't exist, --detect flag, or --scan-ports given
	if *detect || *scanPorts != "" || !fileExists(*configPath) {
		log.Println("Detecting services...")
		if *scanPorts != "" {
			log.Printf("Scanning ports %s for vLLM...", *scanPorts)
		}
		services := discoverAll(*scanPorts)

		if len(services) == 0 {
			log.Println("Warning: no vLLM or Ollama services detected")
		} else {
			for _, s := range services {
				log.Printf("  Found: %s (%s) at %s", s.Name, s.Type, s.URL)
			}
		}
		cfg := &Config{
			Port:     9100,
			Services: services,
		}
		if err := saveConfig(*configPath, cfg); err != nil {
			log.Fatalf("Failed to save config: %v", err)
		}
		log.Printf("Config written to %s", *configPath)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// CLI --scan-ports overrides config for the lifetime of this process
	// (not persisted; set scan_ports in config.yaml to make it permanent).
	effectiveScanPorts := cfg.ScanPorts
	if *scanPorts != "" {
		effectiveScanPorts = *scanPorts
	}

	interval := defaultRescanInterval
	if cfg.RescanInterval != "" {
		d, err := time.ParseDuration(cfg.RescanInterval)
		if err != nil {
			log.Fatalf("Invalid rescan_interval %q: %v", cfg.RescanInterval, err)
		}
		interval = d
	}

	registry := NewRegistry(cfg.Services)
	registry.StartRediscoverLoop(effectiveScanPorts, interval)

	client := &http.Client{Timeout: 3 * time.Second}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", handleStatus(registry, client))
	mux.HandleFunc("GET /gpu", handleGpu)

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	log.Printf("Starting server on %s (%d services configured)", addr, len(cfg.Services))
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
