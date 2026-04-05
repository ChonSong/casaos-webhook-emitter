# CasaOS Webhook Emitter

> **Status:** Phase 1 — Scaffolded. Awaiting MessageBus API research to wire real WebSocket subscription.

A small, long-running Go service that subscribes to the CasaOS-MessageBus WebSocket stream and fans out matching events as HTTP POST requests to registered webhook endpoints.

GitHub: https://github.com/ChonSong/casaos-webhook-emitter

## What It Does

```
CasaOS-MessageBus (WebSocket)
        │
        ▼
┌─────────────────────────┐
│  Webhook Emitter        │
│  • Subscribes to events │
│  • Matches to webhooks  │
│  • Delivers with retry  │
└─────────────────────────┘
        │
        ▼
  Registered Agent Webhooks
```

## Quick Start

```bash
# Build
make build

# Configure
cp webhook-emitter.yaml ~/.config/casaos-agent/webhook-emitter.yaml
nano ~/.config/casaos-agent/webhook-emitter.yaml

# Run
./casaos-webhook-emitter

# Or via systemd (recommended)
systemctl --user enable --now casaos-webhook-emitter
```

## Configuration

`~/.config/casaos-agent/webhook-emitter.yaml`:

```yaml
message_bus:
  url: "http://localhost:8080"
  token: ""           # from CasaOS auth config
  websocket_path: "/v2/message_bus/subscribe/event"

emitter:
  listen: "localhost:9393"
  max_concurrent_deliveries: 10
  delivery_timeout_seconds: 10
  retry_attempts: 3
  retry_backoff_seconds: [1, 5, 30]
  rate_limit_per_minute: 60

webhooks:
  config_path: "~/.config/casaos-agent/webhooks.json"
  hot_reload: true
```

## Webhook Registration

Agents register webhooks via the emitter's management API:

```bash
# Register
curl -X POST http://localhost:9393/webhooks \
  -H "Content-Type: application/json" \
  -d '{"url": "https://agent.example.com/hooks/casaos", "events": ["casaos:file:operate"]}'

# List
curl http://localhost:9393/webhooks

# Delete
curl -X DELETE http://localhost:9393/webhooks/wh_abc123xyz

# Test
curl -X POST http://localhost:9393/webhooks/wh_abc123xyz/test
```

Or use [casaos-agent](https://github.com/ChonSong/casaos-agent):

```bash
casaos-agent webhook register https://agent.example.com/hooks/casaos \
  --event casaos:file:operate \
  --event casaos:system:utilization
```

## Webhook Payload

When an event fires, the emitter POSTs to each matching webhook:

```json
{
  "id": "evt_01J9...",
  "type": "casaos:file:operate",
  "source": "casaos",
  "timestamp": "2026-04-05T17:00:00Z",
  "data": {
    "path": "/DATA/foo.txt",
    "operation": "create",
    "user": "admin"
  }
}
```

With headers:
```
Content-Type: application/json
X-CasaOS-Event: casaos:file:operate
X-CasaOS-Source: casaos
X-CasaOS-Timestamp: 2026-04-05T17:00:00Z
X-CasaOS-Delivery-ID: evt_01J9...
X-CasaOS-Signature: <hmac-sha256 if secret configured>
```

## Retry Policy

- **3 attempts** with exponential backoff: 1s → 5s → 30s
- HTTP 2xx = success
- HTTP 410 Gone = permanent failure → webhook auto-disabled
- Failed deliveries after all retries → dead letter log

## Management API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/webhooks` | List all webhooks |
| `POST` | `/webhooks` | Register a webhook |
| `DELETE` | `/webhooks/:id` | Remove a webhook |
| `GET` | `/webhooks/:id/deliveries` | Delivery history |
| `POST` | `/webhooks/:id/test` | Send test event |
| `GET` | `/health` | Emitter health |
| `GET` | `/metrics` | Prometheus metrics |

## Architecture

```
cmd/emitter/main.go           # Entry point + wire-up
internal/
  bus/        # MessageBus WebSocket client (gorilla/websocket)
  delivery/   # HTTP delivery engine + retries + rate limiting
  registry/  # Webhook registry + JSON persistence
  api/       # Management HTTP server (gorilla/mux)
  config/    # YAML config loading
```

## Systemd Service

`~/.config/systemd/user/casaos-webhook-emitter.service`:

```ini
[Unit]
Description=CasaOS Webhook Emitter
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/casaos-webhook-emitter
Restart=on-failure
RestartSec=5s
Environment=HOME=%h

[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable --now casaos-webhook-emitter
```

## Pair With

- **[casaos-agent](https://github.com/ChonSong/casaos-agent)** — The agent-native CasaOS CLI that includes `webhook` commands to register and manage webhooks.

## License

MIT
