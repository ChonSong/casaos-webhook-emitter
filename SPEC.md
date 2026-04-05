# CasaOS Webhook Emitter — Specification

> GitHub: https://github.com/ChonSong/casaos-webhook-emitter

## Overview

**What it is:** A small, long-running Go service that subscribes to the CasaOS-MessageBus WebSocket stream and fans out matching events as HTTP POST requests to registered webhook endpoints.

**What it replaces:** Nothing — this is net-new infrastructure that enables agents to receive real-time CasaOS event notifications without polling.

**What it pairs with:** `casaos-agent` CLI — agents use the CLI to register webhooks, and the emitter handles delivery.

---

## Architecture

```
CasaOS-MessageBus (WebSocket) 
        │
        ▼
┌─────────────────────────┐
│  Webhook Emitter        │
│  ┌───────────────────┐  │
│  │ MessageBus Client │  │  ← Subscribes to event stream
│  └─────────┬─────────┘  │
│            │              │
│  ┌─────────▼─────────┐  │
│  │ Webhook Registry  │  │  ← In-memory store + JSON config file
│  └─────────┬─────────┘  │
│            │              │
│  ┌─────────▼─────────┐  │
│  │ Delivery Engine   │  │  ← HTTP POST with retries, backoff
│  └───────────────────┘  │
└─────────────────────────┘
        │
        ▼
  Registered Webhook URLs (agents, automations)
```

---

## Event Sources

### CasaOS-MessageBus (WebSocket)
- **Endpoint:** `ws://<message-bus-host>:port/v2/message_bus/subscribe/event`
- **Protocol:** WebSocket with optional `?names=<event-type>` query filter
- **Auth:** Bearer token (from CasaOS config)
- **Events emitted by CasaOS daemon:**
  - `casaos:system:utilization` — periodic hardware stats
  - `casaos:file:operate` — file operation events
  - `casaos:file:recover` — file recovery events

### CasaOS-AppManagement (future integration)
- App install/start/stop/uninstall events (endpoint TBD — research to confirm)
- Likely via same MessageBus or direct Docker events

---

## Webhook Registry

### Registration
Agents register webhooks via:
1. **CLI:** `casaos-agent webhook register https://agent.example.com/hooks/casaos --event casaos:file:operate`
2. **Direct HTTP:** `POST /webhooks` with JSON body
3. **File-based:** `webhooks.json` at startup

### Webhook Record Schema
```json
{
  "id": "wh_01J9...",
  "url": "https://agent.example.com/hooks/casaos",
  "events": ["casaos:file:operate", "casaos:system:utilization"],
  "secret": "optional-hmac-secret",
  "created_at": "2026-04-05T17:00:00Z",
  "enabled": true,
  "filters": {
    "exclude_sources": ["casaos:internal:debug"],
    "match_tags": []
  }
}
```

### Registry Persistence
- Registry stored in `~/.config/casaos-agent/webhooks.json`
- Hot-reload on file change (inotify or polling)

---

## Webhook Delivery

### HTTP Request Format
```
POST <registered-url> HTTP/1.1
Host: <from-url>
Content-Type: application/json
X-CasaOS-Event: <event-name>
X-CasaOS-Source: <source-id>
X-CasaOS-Timestamp: <unix-timestamp>
X-CasaOS-Delivery-ID: <uuid>
X-CasaOS-Signature: <hmac-sha256 if secret set>
```

### Payload
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

### Retry Policy
- **Attempts:** 3
- **Backoff:** exponential, 1s → 5s → 30s
- **Timeout:** 10s per request
- **Success:** HTTP 2xx response
- **Permanent failure:** HTTP 410 Gone → webhook auto-disabled
- **Transient failure:** retry with backoff
- **Dead letter:** Failed deliveries after all retries logged to `~/.local/share/casaos-agent/webhook-emitter/failed deliveries.jsonl`

### Concurrency
- Max 10 concurrent deliveries
- Per-webhook rate limit: 60 deliveries/minute (sliding window)

---

## REST API (for management)

The emitter exposes a local HTTP management port (default `localhost:9393`):

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/webhooks` | List all webhooks |
| `POST` | `/webhooks` | Register a webhook |
| `DELETE` | `/webhooks/:id` | Deregister a webhook |
| `GET` | `/webhooks/:id/deliveries` | Delivery history |
| `POST` | `/webhooks/:id/test` | Send test event |
| `GET` | `/health` | Emitter health |
| `GET` | `/metrics` | Prometheus metrics |

---

## Configuration

Config file: `~/.config/casaos-agent/webhook-emitter.yaml`

```yaml
message_bus:
  url: "http://localhost:8080"  # or UNIX socket path
  token: ""                     # from CasaOS auth config
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

logging:
  level: "info"  # debug, info, warn, error
  format: "json"
```

---

## Operational Modes

### Mode 1: Systemd service (recommended for self-host)
```
~/.config/systemd/user/casaos-webhook-emitter.service
```
Runs as a user-level systemd service. Auto-restarts on failure.

### Mode 2: Docker sidecar
Container runs alongside other CasaOS services. Volume mounts:
- `~/.config/casaos-agent:/config`
- `~/.local/share/casaos-agent:/data`

---

## File Structure

```
casaos-webhook-emitter/
├── cmd/
│   └── emitter/
│       └── main.go
├── internal/
│   ├── bus/           # MessageBus WebSocket client
│   ├── delivery/      # HTTP delivery engine + retries
│   ├── registry/      # Webhook registry + persistence
│   ├── api/           # Management HTTP server
│   └── config/        # Config file loading
├── Makefile
├── README.md
└── SPEC.md
```

---

## Out of Scope (Phase 1)

- MQTT or AMQP transport (only HTTP webhooks)
- Clustering / high availability
-fan-out to more than 100 webhooks per instance
- Persistent delivery queue (SQLite/Postgres) — in-memory only with dead-letter file
