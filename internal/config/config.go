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
}

type MessageBusConfig struct { URL string; Token string; WebsocketPath string }
type EmitterConfig struct {
	Listen                    string
	MaxConcurrentDeliveries  int
	DeliveryTimeoutSeconds   int
	RetryAttempts            int
	RetryBackoffSeconds      []int
	RateLimitPerMinute       int
}
type WebhooksConfig struct { ConfigPath string; HotReload bool }

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(os.ExpandEnv(path))
	if err != nil { return nil, fmt.Errorf("read config: %w", err) }
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil { return nil, fmt.Errorf("parse yaml: %w", err) }
	if cfg.Emitter.MaxConcurrentDeliveries == 0 { cfg.Emitter.MaxConcurrentDeliveries = 10 }
	if cfg.Emitter.DeliveryTimeoutSeconds == 0 { cfg.Emitter.DeliveryTimeoutSeconds = 10 }
	if cfg.Emitter.RetryAttempts == 0 { cfg.Emitter.RetryAttempts = 3 }
	if len(cfg.Emitter.RetryBackoffSeconds) == 0 { cfg.Emitter.RetryBackoffSeconds = []int{1, 5, 30} }
	if cfg.Emitter.RateLimitPerMinute == 0 { cfg.Emitter.RateLimitPerMinute = 60 }
	if cfg.Emitter.Listen == "" { cfg.Emitter.Listen = "localhost:9393" }
	if cfg.Webhooks.ConfigPath == "" { cfg.Webhooks.ConfigPath = "~/.config/casaos-agent/webhooks.json" }
	return &cfg, nil
}
