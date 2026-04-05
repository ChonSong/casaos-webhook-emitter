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
        │  ws://host:port/v2/message_bus/subscribe/event?source_id=casaos-app-management&names=app:install-begin,...
        │
        ▼
┌─────────────────────────┐
│  Webhook Emitter        │
│  ┌───────────────────┐  │
│  │ MessageBus Client │  │  ← gobwas/ws WebSocket, subscribes to event stream
│  └─────────┬─────────┘  │
│            │              │
│  ┌─────────▼─────────┐  │
│  │ Webhook Registry  │  │  ← In-memory + JSON config file
│  └─────────┬─────────┘  │
│            │              │
│  ┌─────────▼─────────┐  │
│  │ Delivery Engine  │  │  ← HTTP POST with retries, backoff, dead-letter
│  └───────────────────┘  │
└─────────────────────────┘
        │
        ▼
  Registered Agent Webhook URLs
```

---

## Event Sources

### CasaOS-MessageBus WebSocket
- **Endpoint:** `ws://<message-bus-host>:port/v2/message_bus/subscribe/event`
- **Query params:** `source_id` (required), `names` (optional comma-separated event names)
- **Protocol:** `gobwas/ws` — text frames carry JSON; ping/pong for keepalive
- **Auth:** Bearer token via `Authorization` header
- **Events delivered as binary WebSocket frames:** `wsutil.WriteServerText(conn, message)` in MessageBus source

### AppManagement Event Catalog (complete, from source)

Source: `CasaOS-AppManagement/common/message.go`

**App lifecycle:**
| Event Name | Source | Properties |
|-----------|--------|------------|
| `app:install-begin` | app-management | `app:name`, `app:icon` |
| `app:install-progress` | app-management | `app:name`, `app:icon`, `app:progress`, `app:title`, `check_port_conflict`, `dry_run` |
| `app:install-end` | app-management | `app:name`, `app:icon` |
| `app:install-error` | app-management | `app:name`, `app:icon`, `message` |
| `app:uninstall-begin` | app-management | `app:name` |
| `app:uninstall-end` | app-management | `app:name` |
| `app:uninstall-error` | app-management | `app:name`, `message` |
| `app:update-begin` | app-management | — |
| `app:update-end` | app-management | — |
| `app:update-error` | app-management | `message` |
| `app:apply-changes-begin` | app-management | — |
| `app:apply-changes-end` | app-management | — |
| `app:apply-changes-error` | app-management | `message` |
| `app:start-begin` | app-management | `app:name` |
| `app:start-end` | app-management | `app:name` |
| `app:start-error` | app-management | `app:name`, `message` |
| `app:stop-begin` | app-management | `app:name` |
| `app:stop-end` | app-management | `app:name` |
| `app:stop-error` | app-management | `app:name`, `message` |
| `app:restart-begin` | app-management | `app:name` |
| `app:restart-end` | app-management | `app:name` |
| `app:restart-error` | app-management | `app:name`, `message` |

**Docker image events:**
| Event Name | Properties |
|-----------|------------|
| `docker:image:pull-begin` | `app:name` |
| `docker:image:pull-progress` | `app:name`, `message` |
| `docker:image:pull-end` | `app:name`, `docker:image:updated` |
| `docker:image:pull-error` | `app:name`, `message` |
| `docker:image:remove-begin` | `app:name` |
| `docker:image:remove-end` | `app:name` |
| `docker:image:remove-error` | `app:name`, `message` |

**Docker container events:**
| Event Name | Properties |
|-----------|------------|
| `docker:container:create-begin` | `docker:container:name` |
| `docker:container:create-end` | `docker:container:id`, `docker:container:name` |
| `docker:container:create-error` | `docker:container:name`, `message` |
| `docker:container:start-begin` | `docker:container:id` |
| `docker:container:start-end` | `docker:container:id` |
| `docker:container:start-error` | `docker:container:id`, `message` |
| `docker:container:stop-begin` | `docker:container:id` |
| `docker:container:stop-end` | `docker:container:id` |
| `docker:container:stop-error` | `docker:container:id`, `message` |
| `docker:container:rename-begin` | `docker:container:id`, `docker:container:name` |
| `docker:container:rename-end` | `docker:container:id`, `docker:container:name` |
| `docker:container:rename-error` | `docker:container:id`, `docker:container:name`, `message` |
| `docker:container:remove-begin` | `docker:container:id` |
| `docker:container:remove-end` | `docker:container:id` |
| `docker:container:remove-error` | `docker:container:id`, `message` |

**CasaOS daemon events:**
| Event Name | Source | Description |
|-----------|--------|-------------|
| `casaos:system:utilization` | casaos | Periodic hardware utilization |
| `casaos:file:operate` | casaos | File operations (copy, move, delete) |
| `casaos:file:recover` | casaos | File recovery events |

---

## WebSocket Message Format

The MessageBus sends JSON events as binary WebSocket frames:

```json
{
  "source_id": "casaos-app-management",
  "name": "app:install-progress",
  "properties": {
    "app:name": "homeassistant/homeassistant",
    "app:icon": "https://example.com/icon.png",
    "app:progress": "64",
    "app:title": "{\"en_us\":\"Home Assistant\"}"
  },
  "timestamp": 1743865200,
  "uuid": "evt_01J9..."
}
```

---

## Webhook Registry

### Registration
Agents register webhooks via:
1. **CLI:** `casaos-agent webhook register https://agent.example.com/hooks --event app:install-end`
2. **Direct HTTP:** `POST http://localhost:9393/webhooks` with JSON body
3. **File-based:** `webhooks.json` at startup

### Webhook Record Schema
```json
{
  "id": "wh_01J9...",
  "url": "https://agent.example.com/hooks",
  "events": ["app:install-end", "app:install-error", "docker:container:stop-end"],
  "secret": "",
  "created_at": "2026-04-05T17:00:00Z",
  "enabled": true
}
```

### Registry Persistence
- Registry stored in `~/.config/casaos-agent/webhooks.json`
- Hot-reload via polling

---

## Webhook Delivery

### HTTP Request Format
```
POST <registered-url> HTTP/1.1
Host: <from-url>
Content-Type: application/json
X-CasaOS-Event: <event-name>
X-CasaOS-Source: <source_id>
X-CasaOS-Timestamp: <unix-timestamp>
X-CasaOS-Delivery-ID: <uuid>
X-CasaOS-Signature: <hmac-sha256 if secret set>
```

### Payload
```json
{
  "id": "evt_01J9...",
  "type": "app:install-progress",
  "source": "casaos-app-management",
  "timestamp": "2026-04-05T17:00:00Z",
  "properties": {
    "app:name": "homeassistant/homeassistant",
    "app:progress": "64"
  }
}
```

### Retry Policy
- **Attempts:** 3
- **Backoff:** exponential, 1s → 5s → 30s
- **Timeout:** 10s per request
- **Success:** HTTP 2xx
- **Permanent failure:** HTTP 410 → webhook auto-disabled
- **Dead letter:** `~/.local/share/casaos-agent/webhook-emitter/failed_deliveries.jsonl`

### Concurrency
- Max 10 concurrent deliveries
- Per-webhook rate limit: 60 deliveries/minute

---

## REST API (Management Port)

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

`~/.config/casaos-agent/webhook-emitter.yaml`:

```yaml
message_bus:
  url: "http://localhost:8080"
  token: ""
  websocket_path: "/v2/message_bus/subscribe/event"
  source_id: "casaos-agent"  # source ID we subscribe as

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

---

## File Structure

```
casaos-webhook-emitter/
├── cmd/emitter/main.go
├── internal/
│   ├── bus/           # MessageBus WebSocket client (gobwas/ws)
│   ├── delivery/      # HTTP delivery engine + retries + dead-letter
│   ├── registry/     # Webhook registry + JSON persistence
│   ├── api/          # Management HTTP server (gorilla/mux)
│   └── config/       # YAML config loading
├── Makefile
└── SPEC.md
```
