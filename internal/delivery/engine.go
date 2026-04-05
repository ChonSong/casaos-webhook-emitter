package delivery

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chonSong/casaos-webhook-emitter/internal/bus"
	"github.com/chonSong/casaos-webhook-emitter/internal/registry"
)

type WebhookPayload struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Source    string      `json:"source"`
	Timestamp string      `json:"timestamp"`
	Data      interface{} `json:"data"`
}

type DeliveryResult struct {
	WebhookID string `json:"webhook_id"`
	EventID   string `json:"event_id"`
	Attempt   int    `json:"attempt"`
	Status    int    `json:"status"`
	Error     string `json:"error,omitempty"`
	Timestamp string `json:"timestamp"`
}

type Engine struct {
	cfg       Config
	sem       chan struct{}
	client    http.Client
	results   []DeliveryResult
	historyMu sync.RWMutex
}

type Config struct {
	MaxConcurrent    int
	TimeoutSeconds  int
	RetryAttempts   int
	RetryBackoffSecs []int
	RateLimitPerMin int
}

func NewEngine(cfg Config) *Engine {
	if cfg.MaxConcurrent == 0 { cfg.MaxConcurrent = 10 }
	if cfg.TimeoutSeconds == 0 { cfg.TimeoutSeconds = 10 }
	if cfg.RetryAttempts == 0 { cfg.RetryAttempts = 3 }
	if len(cfg.RetryBackoffSecs) == 0 { cfg.RetryBackoffSecs = []int{1, 5, 30} }
	return &Engine{cfg: cfg, sem: make(chan struct{}, cfg.MaxConcurrent),
		client: http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}}
}

func (e *Engine) Deliver(webhook registry.Webhook, event bus.Event) {
	e.sem <- struct{}{}
	go func() { defer func() { <-e.sem }(); e.deliverWithRetry(webhook, event) }()
}

func (e *Engine) deliverWithRetry(webhook registry.Webhook, event bus.Event) {
	ts := time.Unix(event.Timestamp, 0).UTC().Format(time.RFC3339)
	payload := WebhookPayload{ID: event.UUID, Type: event.Name, Source: event.SourceID, Timestamp: ts, Data: event.Properties}
	body, _ := json.Marshal(payload)
	for attempt := 1; attempt <= e.cfg.RetryAttempts; attempt++ {
		status, err := e.doDelivery(webhook, body, event.UUID)
		result := DeliveryResult{WebhookID: webhook.ID, EventID: event.UUID, Attempt: attempt, Status: status, Timestamp: time.Now().UTC().Format(time.RFC3339)}
		if err != nil { result.Error = err.Error(); log.Printf("[delivery] attempt %d/%d failed for %s: %v", attempt, e.cfg.RetryAttempts, webhook.URL, err)
		} else { log.Printf("[delivery] success %s -> %s (attempt %d)", event.Name, webhook.URL, attempt) }
		e.recordResult(result)
		if err == nil || status == 410 { if status == 410 { log.Printf("[delivery] permanent failure (410), disabling webhook %s", webhook.ID) }; return }
		if attempt < len(e.cfg.RetryBackoffSecs) { time.Sleep(time.Duration(e.cfg.RetryBackoffSecs[attempt-1]) * time.Second) }
	}
	e.logDeadLetter(webhook, payload)
}

func (e *Engine) doDelivery(webhook registry.Webhook, body []byte, eventID string) (int, error) {
	req, _ := http.NewRequest(http.MethodPost, webhook.URL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CasaOS-Event", webhook.ID)
	req.Header.Set("X-CasaOS-Timestamp", time.Now().UTC().Format(time.RFC3339))
	req.Header.Set("X-CasaOS-Delivery-ID", eventID)
	if webhook.Secret != "" { mac := hmac.New(sha256.New, []byte(webhook.Secret)); mac.Write(body); req.Header.Set("X-CasaOS-Signature", hex.EncodeToString(mac.Sum(nil))) }
	resp, err := e.client.Do(req)
	if err != nil { return 0, fmt.Errorf("request: %w", err) }
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func (e *Engine) recordResult(r DeliveryResult) {
	e.historyMu.Lock()
	e.results = append(e.results, r)
	if len(e.results) > 1000 { e.results = e.results[len(e.results)-1000:] }
	e.historyMu.Unlock()
}

func (e *Engine) GetHistory(webhookID string) []DeliveryResult {
	e.historyMu.RLock()
	defer e.historyMu.RUnlock()
	var out []DeliveryResult
	for _, r := range e.results { if r.WebhookID == webhookID { out = append(out, r) } }
	return out
}

func (e *Engine) logDeadLetter(webhook registry.Webhook, payload WebhookPayload) {
	dir := filepath.Join(os.Getenv("HOME"), ".local", "share", "casaos-agent", "webhook-emitter")
	os.MkdirAll(dir, 0755)
	f, err := os.OpenFile(filepath.Join(dir, "failed_deliveries.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil { log.Printf("[delivery] dead letter error: %v", err); return }
	defer f.Close()
	rec := map[string]interface{}{"webhook_id": webhook.ID, "webhook_url": webhook.URL, "payload": payload, "ts": time.Now().UTC().Format(time.RFC3339)}
	json.NewEncoder(f).Encode(rec)
	log.Printf("[delivery] dead letter for webhook %s", webhook.ID)
}
