package bus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"nhooyr.io/websocket"
)

type Event struct {
	SourceID   string            `json:"source_id"`
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
	Timestamp  int64             `json:"timestamp"`
	UUID       string            `json:"uuid"`
}

type Client struct {
	baseURL string
	token   string
	handler func(Event)
}

func NewClient(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &Client{baseURL: baseURL, token: token}
}

func (c *Client) OnEvent(handler func(Event)) { c.handler = handler }

func (c *Client) Subscribe(ctx context.Context, path string) error {
	u, _ := url.Parse(c.baseURL)
	if u.Scheme == "http" {
		u.Scheme = "ws"
	}
	u.Path = path
	headers := http.Header{}
	if c.token != "" {
		headers.Set("Authorization", "Bearer "+c.token)
	}
	conn, _, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		HTTPHeader: headers,
	})
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}
	defer conn.Close(websocket.StatusGoingAway, "")
	log.Printf("[bus] connected to MessageBus at %s", u.String())
	go func() {
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				conn.Ping(ctx)
			}
		}
	}()
	for {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("[bus] read error: %v", err)
			return err
		}
		var event Event
		if err := json.Unmarshal(msg, &event); err != nil {
			log.Printf("[bus] unmarshal error: %v", err)
			continue
		}
		if c.handler != nil {
			c.handler(event)
		}
	}
}
