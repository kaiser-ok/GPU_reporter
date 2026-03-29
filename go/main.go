package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	detect := flag.Bool("detect", false, "force re-detect services and regenerate config")
	flag.Parse()

	// Auto-detect if config doesn't exist or --detect flag
	if *detect || !fileExists(*configPath) {
		log.Println("Detecting services...")
		services := detectServices()
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

	client := &http.Client{Timeout: 3 * time.Second}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", handleStatus(cfg, client))
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
