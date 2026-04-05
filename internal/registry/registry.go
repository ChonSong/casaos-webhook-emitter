package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

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
	path     string
	mu       sync.RWMutex
	webhooks []Webhook
}

func New(path string) (*Registry, error) {
	r := &Registry{path: os.ExpandEnv(path)}
	if err := r.load(); err != nil { return nil, err }
	return r, nil
}

func (r *Registry) load() error {
	data, err := os.ReadFile(r.path)
	if err != nil { if os.IsNotExist(err) { r.webhooks = []Webhook{}; return nil }; return fmt.Errorf("read webhooks: %w", err) }
	if err := json.Unmarshal(data, &r.webhooks); err != nil { return fmt.Errorf("parse webhooks.json: %w", err) }
	return nil
}

func (r *Registry) List() []Webhook { r.mu.RLock(); defer r.mu.RUnlock(); out := make([]Webhook, len(r.webhooks)); copy(out, r.webhooks); return out }
func (r *Registry) Get(id string) *Webhook { r.mu.RLock(); defer r.mu.RUnlock(); for _, w := range r.webhooks { if w.ID == id { return &w } }; return nil }
func (r *Registry) Add(wh Webhook) error { r.mu.Lock(); r.webhooks = append(r.webhooks, wh); r.mu.Unlock(); return r.save() }
func (r *Registry) Remove(id string) bool { r.mu.Lock(); defer r.mu.Unlock(); before := len(r.webhooks); r.webhooks = func() []Webhook { o := []Webhook{}; for _, w := range r.webhooks { if w.ID != id { o = append(o, w) } }; return o }(); return len(r.webhooks) == before-1 }

func (r *Registry) MatchingWebhooks(eventName string) []Webhook {
	r.mu.RLock(); defer r.mu.RUnlock()
	var out []Webhook
	for _, wh := range r.webhooks {
		if !wh.Enabled { continue }
		if len(wh.Events) == 0 { out = append(out, wh); continue }
		for _, ev := range wh.Events { if ev == eventName || ev == "*" { out = append(out, wh); break } }
	}
	return out
}

func (r *Registry) save() error {
	r.mu.RLock(); defer r.mu.RUnlock()
	data, _ := json.MarshalIndent(r.webhooks, "", "  ")
	dir := r.path[:len(r.path)-len("/webhooks.json")]; os.MkdirAll(dir, 0755)
	return os.WriteFile(r.path, data, 0644)
}
