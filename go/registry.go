package main

import (
	"log"
	"sync"
	"time"
)

// Registry holds the live set of monitored services. It is safe for
// concurrent access: the background rediscover loop may rewrite the list
// while /status handlers are reading a snapshot.
//
// Services loaded from config.yaml are "pinned" — the rediscover loop never
// removes them, even if they go unreachable, because they represent explicit
// user intent. Services found later by auto-detection are dynamic: they're
// added when first seen and removed when a rediscover pass stops finding them.
type Registry struct {
	mu       sync.RWMutex
	services []ServiceConfig
	pinned   map[string]bool // URLs loaded from config at startup
}

func NewRegistry(initial []ServiceConfig) *Registry {
	pinned := make(map[string]bool, len(initial))
	for _, s := range initial {
		pinned[s.URL] = true
	}
	cpy := make([]ServiceConfig, len(initial))
	copy(cpy, initial)
	return &Registry{services: cpy, pinned: pinned}
}

// Snapshot returns a copy of the current service list, safe to iterate
// without holding the registry lock.
func (r *Registry) Snapshot() []ServiceConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ServiceConfig, len(r.services))
	copy(out, r.services)
	return out
}

// Rediscover merges a fresh detection result into the live set. Pinned
// services are always retained; previously-discovered services no longer
// present in `found` are dropped and their delta state cleared.
func (r *Registry) Rediscover(found []ServiceConfig) (added, removed int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	foundURLs := make(map[string]bool, len(found))
	for _, s := range found {
		foundURLs[s.URL] = true
	}

	kept := make([]ServiceConfig, 0, len(r.services)+len(found))
	keptURLs := make(map[string]bool, len(r.services))
	for _, s := range r.services {
		if r.pinned[s.URL] || foundURLs[s.URL] {
			kept = append(kept, s)
			keptURLs[s.URL] = true
		} else {
			removed++
			resetVllmState(s.URL)
		}
	}

	for _, s := range found {
		if !keptURLs[s.URL] {
			kept = append(kept, s)
			keptURLs[s.URL] = true
			added++
		}
	}

	r.services = kept
	return added, removed
}

// StartRediscoverLoop runs discoverAll on a fixed interval and applies the
// result to the registry. Interval <= 0 disables the loop (returns immediately).
func (r *Registry) StartRediscoverLoop(scanPorts string, interval time.Duration) {
	if interval <= 0 {
		log.Println("Rediscover loop disabled (rescan_interval <= 0)")
		return
	}
	log.Printf("Rediscover loop starting (interval=%s, scan_ports=%q)", interval, scanPorts)
	go func() {
		tick := time.NewTicker(interval)
		defer tick.Stop()
		for range tick.C {
			found := discoverAll(scanPorts)
			added, removed := r.Rediscover(found)
			if added > 0 || removed > 0 {
				log.Printf("Rediscover: +%d -%d (total %d)", added, removed, len(r.Snapshot()))
			}
		}
	}()
}
