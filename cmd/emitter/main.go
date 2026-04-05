package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/chonSong/casaos-webhook-emitter/internal/api"
	"github.com/chonSong/casaos-webhook-emitter/internal/bus"
	"github.com/chonSong/casaos-webhook-emitter/internal/config"
	"github.com/chonSong/casaos-webhook-emitter/internal/delivery"
	"github.com/chonSong/casaos-webhook-emitter/internal/registry"
)

func main() {
	cfg, err := config.Load("~/.config/casaos-agent/webhook-emitter.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Init registry (loads webhooks.json)
	reg, err := registry.New(cfg.Webhooks.ConfigPath)
	if err != nil {
		log.Fatalf("failed to init registry: %v", err)
	}

	// Delivery engine
	engine := delivery.NewEngine(delivery.Config{
		MaxConcurrent:    cfg.Emitter.MaxConcurrentDeliveries,
		TimeoutSeconds:   cfg.Emitter.DeliveryTimeoutSeconds,
		RetryAttempts:     cfg.Emitter.RetryAttempts,
		RetryBackoffSecs:  cfg.Emitter.RetryBackoffSeconds,
		RateLimitPerMin:   cfg.Emitter.RateLimitPerMinute,
	})

	// MessageBus WebSocket client
	client, err := bus.NewClient(cfg.MessageBus.URL, cfg.MessageBus.Token)
	if err != nil {
		log.Fatalf("failed to connect to MessageBus: %v", err)
	}

	// Wire: MessageBus events → delivery engine → registered webhooks
	client.OnEvent(func(event bus.Event) {
		hooks := reg.MatchingWebhooks(event.Type)
		for _, hook := range hooks {
			engine.Deliver(hook, event)
		}
	})

	// Management API server
	apiServer := api.New(reg, engine, cfg.Emitter.Listen)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start WebSocket subscription
	go func() {
		if err := client.Subscribe(ctx, cfg.MessageBus.WebsocketPath); err != nil {
			log.Printf("MessageBus subscription error: %v", err)
		}
	}()

	// Start management API
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Printf("API server error: %v", err)
		}
	}()

	// Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	cancel()
	engine.Close()
	log.Println("casaos-webhook-emitter stopped")
}
