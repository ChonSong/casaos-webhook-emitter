package bus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// Event is a CasaOS event received from the MessageBus
type Event struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	SourceID  string          `json:"source_id"`
	Timestamp string          `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// Client subscribes to CasaOS-MessageBus WebSocket and delivers events to a callback
type Client struct {
	baseURL  string
	token    string
	dialer   websocket.Dialer
	mu       chan struct{}
	conn     *websocket.Conn
	handler  func(Event)
}

func NewClient(baseURL, token string) (*Client, error) {
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		dialer: websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		},
		mu: make(chan struct{}, 1),
	}, nil
}

func (c *Client) OnEvent(handler func(Event)) {
	c.mu <- struct{}{} // acquire
	c.handler = handler
	<-c.mu // release
}

func (c *Client) Subscribe(ctx context.Context, path string) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	if u.Scheme == "http" {
		u.Scheme = "ws"
	}
	u.Path = path

	headers := http.Header{}
	if c.token != "" {
		headers.Set("Authorization", "Bearer "+c.token)
	}

	conn, _, err := c.dialer.Dial(u.String(), headers)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}
	defer conn.Close()

	c.mu <- struct{}{}
	c.conn = conn
	<-c.mu

	log.Printf("[bus] Connected to MessageBus at %s", u.String())

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					log.Printf("[bus] WebSocket read error: %v", err)
				}
				return err
			}
			c.handleMessage(msg)
		}
	}
}

func (c *Client) handleMessage(msg []byte) {
	// Try wrapper format: { "event": { ... } }
	var wrapper struct {
		Event Event `json:"event"`
	}
	if err := json.Unmarshal(msg, &wrapper); err == nil && wrapper.Event.Name != "" {
		c.deliver(wrapper.Event)
		return
	}

	// Try direct event format
	var event Event
	if err := json.Unmarshal(msg, &event); err == nil && event.Name != "" {
		c.deliver(event)
		return
	}

	// Ignore unknown formats (heartbeats, pings, etc.)
}

func (c *Client) deliver(event Event) {
	c.mu <- struct{}{}
	h := c.handler
	<-c.mu
	if h != nil {
		h(event)
	}
}
