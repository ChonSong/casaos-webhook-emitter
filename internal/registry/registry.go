package registry

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
)

// Webhook represents a registered webhook
type Webhook struct {
	ID        string            `json:"id"`
	URL       string            `json:"url"`
	Events    []string          `json:"events"`
	Secret    string            `json:"secret,omitempty"`
	Enabled   bool              `json:"enabled"`
	CreatedAt string            `json:"created_at"`
	Filters   map[string]string `json:"filters,omitempty"`
}

type Registry struct {
	path      string
	mu        sync.RWMutex
	webhooks  []Webhook
	hotReload bool
}

func New(path string) (*Registry, error) {
	expanded := os.ExpandEnv(path)
	r := &Registry{path: expanded, hotReload: true}
	if err := r.load(); err != nil {
		return nil, err
	}
	if r.hotReload {
		go r.watch()
	}
	return r, nil
}

func (r *Registry) load() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			r.mu.Lock()
			r.webhooks = []Webhook{}
			r.mu.Unlock()
			return nil
		}
		return fmt.Errorf("read webhooks file: %w", err)
	}
	var webhooks []Webhook
	if err := json.Unmarshal(data, &webhooks); err != nil {
		return fmt.Errorf("parse webhooks.json: %w", err)
	}
	r.mu.Lock()
	r.webhooks = webhooks
	r.mu.Unlock()
	return nil
}

func (r *Registry) watch() {
	// Simple polling approach — a real impl would use inotify
	for {
		r.mu.RLock()
		// poll not implemented — hot reload is file-based via API
		r.mu.RUnlock()
		break
	}
}

func (r *Registry) List() []Webhook {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Webhook, len(r.webhooks))
	copy(out, r.webhooks)
	return out
}

func (r *Registry) Get(id string) *Webhook {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, wh := range r.webhooks {
		if wh.ID == id {
			return &wh
		}
	}
	return nil
}

func (r *Registry) Add(webhook Webhook) error {
	r.mu.Lock()
	r.webhooks = append(r.webhooks, webhook)
	r.mu.Unlock()
	return r.save()
}

func (r *Registry) Remove(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	before := len(r.webhooks)
	r.webhooks = func() []Webhook {
		out := []Webhook{}
		for _, wh := range r.webhooks {
			if wh.ID != id {
				out = append(out, wh)
			}
		}
		return out
	}()
	return len(r.webhooks) == before-1
}

func (r *Registry) MatchingWebhooks(eventName string) []Webhook {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Webhook
	for _, wh := range r.webhooks {
		if !wh.Enabled {
			continue
		}
		if len(wh.Events) == 0 {
			out = append(out, wh)
			continue
		}
		for _, ev := range wh.Events {
			if ev == eventName || ev == "*" {
				out = append(out, wh)
				break
			}
		}
	}
	return out
}

func (r *Registry) save() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	data, err := json.MarshalIndent(r.webhooks, "", "  ")
	if err != nil {
		return err
	}
	// Ensure directory exists
	dir := r.path[:len(r.path)-len("/webhooks.json")]
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(r.path, data, 0644); err != nil {
		return err
	}
	log.Printf("[registry] saved %d webhooks to %s", len(r.webhooks), r.path)
	return nil
}
