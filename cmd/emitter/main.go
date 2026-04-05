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
	cfg, err := config.Load("/home/osboxes/.config/casaos-agent/webhook-emitter.yaml")
	if err != nil { log.Fatalf("config load: %v", err) }
	reg, err := registry.New(os.ExpandEnv(cfg.Webhooks.ConfigPath))
	if err != nil { log.Fatalf("registry load: %v", err) }
	engine := delivery.NewEngine(delivery.Config{
		MaxConcurrent:    cfg.Emitter.MaxConcurrentDeliveries,
		TimeoutSeconds:   cfg.Emitter.DeliveryTimeoutSeconds,
		RetryAttempts:   cfg.Emitter.RetryAttempts,
		RetryBackoffSecs: cfg.Emitter.RetryBackoffSeconds,
		RateLimitPerMin: cfg.Emitter.RateLimitPerMinute,
	})
	client := bus.NewClient(cfg.MessageBus.URL, cfg.MessageBus.Token)
	client.OnEvent(func(event bus.Event) {
		for _, h := range reg.MatchingWebhooks(event.Name) { engine.Deliver(h, event) }
	})
	apiServer := api.New(reg, engine, cfg.Emitter.Listen)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := client.Subscribe(ctx, cfg.MessageBus.WebsocketPath); err != nil { log.Printf("[bus] subscribe: %v", err) }
	}()
	go func() {
		if err := apiServer.Start(); err != nil { log.Printf("[api] server: %v", err) }
	}()
	sig := make(chan os.Signal, 1); signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM); <-sig
	cancel()
}
