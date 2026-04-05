package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	MessageBus MessageBusConfig `yaml:"message_bus"`
	Emitter    EmitterConfig    `yaml:"emitter"`
	Webhooks   WebhooksConfig   `yaml:"webhooks"`
	Logging    LoggingConfig    `yaml:"logging"`
}

type MessageBusConfig struct {
	URL           string `yaml:"url"`             // e.g. "http://localhost:8080"
	Token         string `yaml:"token"`           // Bearer token
	WebsocketPath string `yaml:"websocket_path"`   // e.g. "/v2/message_bus/subscribe/event"
}

type EmitterConfig struct {
	Listen                  string  `yaml:"listen"`                           // "localhost:9393"
	MaxConcurrentDeliveries int     `yaml:"max_concurrent_deliveries"`        // default 10
	DeliveryTimeoutSeconds  int     `yaml:"delivery_timeout_seconds"`        // default 10
	RetryAttempts           int     `yaml:"retry_attempts"`                   // default 3
	RetryBackoffSeconds     []int   `yaml:"retry_backoff_seconds"`           // [1, 5, 30]
	RateLimitPerMinute      int     `yaml:"rate_limit_per_minute"`           // default 60
}

type WebhooksConfig struct {
	ConfigPath string `yaml:"config_path"` // path to webhooks.json
	HotReload  bool   `yaml:"hot_reload"`  // watch file for changes
}

type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // json, text
}

func Load(path string) (*Config, error) {
	expanded := os.ExpandEnv(path)
	data, err := os.ReadFile(expanded)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	// Set defaults
	if cfg.Emitter.MaxConcurrentDeliveries == 0 {
		cfg.Emitter.MaxConcurrentDeliveries = 10
	}
	if cfg.Emitter.DeliveryTimeoutSeconds == 0 {
		cfg.Emitter.DeliveryTimeoutSeconds = 10
	}
	if cfg.Emitter.RetryAttempts == 0 {
		cfg.Emitter.RetryAttempts = 3
	}
	if len(cfg.Emitter.RetryBackoffSeconds) == 0 {
		cfg.Emitter.RetryBackoffSeconds = []int{1, 5, 30}
	}
	if cfg.Emitter.RateLimitPerMinute == 0 {
		cfg.Emitter.RateLimitPerMinute = 60
	}
	if cfg.Emitter.Listen == "" {
		cfg.Emitter.Listen = "localhost:9393"
	}
	return &cfg, nil
}
