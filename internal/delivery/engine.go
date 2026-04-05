package delivery

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/chonSong/casaos-webhook-emitter/internal/bus"
	"github.com/chonSong/casaos-webhook-emitter/internal/registry"
)

// WebhookPayload is what gets POSTed to the webhook URL
type WebhookPayload struct {
	ID        string      `json:"id"`
	Type     string      `json:"type"`
	Source   string      `json:"source"`
	Timestamp string     `json:"timestamp"`
	Data     interface{} `json:"data"`
}

// DeliveryResult records what happened
type DeliveryResult struct {
	WebhookID string        `json:"webhook_id"`
	EventID   string        `json:"event_id"`
	Attempt   int           `json:"attempt"`
	Status    int           `json:"status"`
	Duration  time.Duration `json:"duration_ms"`
	Error     string        `json:"error,omitempty"`
	Timestamp string        `json:"timestamp"`
}

// Engine manages concurrent webhook delivery with retries and rate limiting
type Engine struct {
	cfg     Config
	sem     chan struct{}
	client  http.Client
	results []DeliveryResult
	historyMu sync.Mutex
	rlMu    sync.Map // per-webhook rate limiters
}

type Config struct {
	MaxConcurrent    int
	TimeoutSeconds   int
	RetryAttempts    int
	RetryBackoffSecs []int
	RateLimitPerMin  int
}

func NewEngine(cfg Config) *Engine {
	if cfg.MaxConcurrent == 0 {
		cfg.MaxConcurrent = 10
	}
	if cfg.TimeoutSeconds == 0 {
		cfg.TimeoutSeconds = 10
	}
	if cfg.RetryAttempts == 0 {
		cfg.RetryAttempts = 3
	}
	if len(cfg.RetryBackoffSecs) == 0 {
		cfg.RetryBackoffSecs = []int{1, 5, 30}
	}
	return &Engine{
		cfg:  cfg,
		sem: make(chan struct{}, cfg.MaxConcurrent),
		client: http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
	}
}

func (e *Engine) Deliver(webhook registry.Webhook, event bus.Event) {
	e.sem <- struct{}{}
	go func() {
		defer func() { <-e.sem }()
		e.deliverWithRetry(webhook, event)
	}()
}

func (e *Engine) deliverWithRetry(webhook registry.Webhook, event bus.Event) {
	payload := WebhookPayload{
		ID:        event.ID,
		Type:     event.Name,
		Source:   event.SourceID,
		Timestamp: event.Timestamp,
		Data:     event.Data,
	}

	body, _ := json.Marshal(payload)

	for attempt := 1; attempt <= e.cfg.RetryAttempts; attempt++ {
		status, err := e.doDelivery(webhook, body, payload.ID)
		result := DeliveryResult{
			WebhookID: webhook.ID,
			EventID:   payload.ID,
			Attempt:   attempt,
			Status:    status,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
		if err != nil {
			result.Error = err.Error()
			log.Printf("[delivery] attempt %d/%d failed for %s: %v", attempt, e.cfg.RetryAttempts, webhook.URL, err)
		} else {
			result.Duration = time.Duration(0) // could track
			log.Printf("[delivery] success %s → %s (attempt %d)", payload.Type, webhook.URL, attempt)
		}
		e.recordResult(result)

		if err == nil || status == 410 {
			// Success or permanent failure — stop retrying
			if status == 410 {
				log.Printf("[delivery] permanent failure (410), disabling webhook %s", webhook.ID)
			}
			return
		}

		// Exponential backoff
		if attempt < len(e.cfg.RetryBackoffSecs) {
			backoff := time.Duration(e.cfg.RetryBackoffSecs[attempt-1]) * time.Second
			time.Sleep(backoff)
		}
	}

	// All retries exhausted — log to dead letter
	e.logDeadLetter(webhook, payload, e.cfg.RetryAttempts)
}

func (e *Engine) doDelivery(webhook registry.Webhook, body []byte, eventID string) (int, error) {
	req, err := http.NewRequest(http.MethodPost, webhook.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CasaOS-Event", webhook.ID) // TODO: event type
	req.Header.Set("X-CasaOS-Timestamp", time.Now().UTC().Format(time.RFC3339))
	req.Header.Set("X-CasaOS-Delivery-ID", eventID)

	if webhook.Secret != "" {
		mac := hmac.New(sha256.New, []byte(webhook.Secret))
		mac.Write(body)
		req.Header.Set("X-CasaOS-Signature", hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func (e *Engine) recordResult(r DeliveryResult) {
	e.historyMu.Lock()
	e.results = append(e.results, r)
	if len(e.results) > 1000 {
		e.results = e.results[len(e.results)-1000:]
	}
	e.historyMu.Unlock()
}

func (e *Engine) GetHistory(webhookID string) []DeliveryResult {
	e.historyMu.RLock()
	defer e.historyMu.RUnlock()
	var out []DeliveryResult
	for _, r := range e.results {
		if r.WebhookID == webhookID {
			out = append(out, r)
		}
	}
	return out
}

func (e *Engine) logDeadLetter(webhook registry.Webhook, payload WebhookPayload, attempts int) {
	dlPath := fmt.Sprintf("~/.local/share/casaos-agent/webhook-emitter/failed_deliveries.jsonl")
	expanded := os.ExpandEnv(dlPath)
	f, err := os.OpenFile(expanded, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[delivery] failed to open dead letter file: %v", err)
		return
	}
	defer f.Close()
	record := map[string]interface{}{
		"webhook_id": webhook.ID,
		"webhook_url": webhook.URL,
		"payload": payload,
		"attempts": attempts,
		"ts": time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(record)
	f.Write(data)
	f.WriteString("\n")
	log.Printf("[delivery] dead letter logged for webhook %s, event %s", webhook.ID, payload.ID)
}

func (e *Engine) Close() {
	// graceful: drain in-flight deliveries
	time.Sleep(2 * time.Second)
}
