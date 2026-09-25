package mirasim

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client interacts with the Mirasim backend over WebSocket.
type Client struct {
	wsURL string
	mu    sync.RWMutex
}

// NewClient creates a new Mirasim WebSocket client.
func NewClient(wsURL string) *Client {
	return &Client{
		wsURL: wsURL,
	}
}

// UpdateURL updates the backend WebSocket URL.
func (c *Client) UpdateURL(wsURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wsURL = wsURL
}

// GetURL returns the current WebSocket URL.
func (c *Client) GetURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.wsURL
}

// TurnOptions specifies parameters for a prompt turn.
type TurnOptions struct {
	Prompt     string
	SessionKey string
	Agent      string
	Model      string
	Effort     string
	Workdir    string
}

// TurnEvent represents an incremental event during turn execution.
type TurnEvent struct {
	SessionKey      string
	AppendText      string
	AppendReasoning string
	Usage           *Usage
	Done            bool
	Error           error
}

// GetAgents connects to Mirasim and retrieves the list of supported agents.
func (c *Client) GetAgents(ctx context.Context) ([]AgentInfo, error) {
	u := c.GetURL()
	if u == "" {
		return nil, fmt.Errorf("no Mirasim WebSocket URL configured")
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	header := make(http.Header)
	conn, _, err := dialer.DialContext(ctx, u, header)
	if err != nil {
		return nil, fmt.Errorf("failed to dial mirasim websocket: %w", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]string{"type": "ready"}); err != nil {
		return nil, fmt.Errorf("failed to send ready: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("read error while awaiting init: %w", err)
		}

		var frame struct {
			Type   string      `json:"type"`
			Agents []AgentInfo `json:"agents"`
		}
		if err := json.Unmarshal(msg, &frame); err == nil && frame.Type == "init" {
			return frame.Agents, nil
		}
	}
}

// ExecuteTurn starts a prompt session and streams back events via a channel.
func (c *Client) ExecuteTurn(ctx context.Context, opts TurnOptions) (<-chan TurnEvent, error) {
	u := c.GetURL()
	if u == "" {
		return nil, fmt.Errorf("no Mirasim WebSocket URL configured")
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	header := make(http.Header)
	conn, _, err := dialer.DialContext(ctx, u, header)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mirasim websocket: %w", err)
	}

	events := make(chan TurnEvent, 64)

	go func() {
		defer conn.Close()
		defer close(events)

		clientRef := fmt.Sprintf("cpa-%d", time.Now().UnixNano())

		// Send ready frame first
		if err := conn.WriteJSON(map[string]string{"type": "ready"}); err != nil {
			events <- TurnEvent{Error: fmt.Errorf("failed to send ready frame: %w", err)}
			return
		}

		// Await init
		gotInit := false
		for !gotInit {
			select {
			case <-ctx.Done():
				events <- TurnEvent{Error: ctx.Err()}
				return
			default:
			}

			_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				events <- TurnEvent{Error: fmt.Errorf("error reading init frame: %w", err)}
				return
			}

			var baseFrame struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(msg, &baseFrame); err == nil && baseFrame.Type == "init" {
				gotInit = true
			}
		}

		// Prepare and send prompt frame
		promptFrame := ClientPromptFrame{
			Type:       "prompt",
			ClientRef:  clientRef,
			SessionKey: opts.SessionKey,
			Agent:      opts.Agent,
			Model:      opts.Model,
			Effort:     opts.Effort,
			Prompt:     opts.Prompt,
			Workdir:    opts.Workdir,
		}

		if err := conn.WriteJSON(promptFrame); err != nil {
			events <- TurnEvent{Error: fmt.Errorf("failed to send prompt frame: %w", err)}
			return
		}

		var sessionKey string
		subscribed := false

		stopChan := make(chan struct{})
		defer close(stopChan)

		// Handle context cancellation
		go func() {
			select {
			case <-ctx.Done():
				if sessionKey != "" {
					log.Printf("[Mirasim Client] Context cancelled, sending stop for session %s", sessionKey)
					_ = conn.WriteJSON(ClientStopFrame{
						Type:       "stop",
						SessionKey: sessionKey,
					})
				}
			case <-stopChan:
			}
		}()

		for {
			_ = conn.SetReadDeadline(time.Now().Add(180 * time.Second))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				select {
				case <-ctx.Done():
					events <- TurnEvent{Error: ctx.Err()}
				default:
					events <- TurnEvent{Error: fmt.Errorf("websocket read error: %w", err)}
				}
				return
			}

			var raw map[string]interface{}
			if err := json.Unmarshal(msg, &raw); err != nil {
				continue
			}

			frameType, _ := raw["type"].(string)

			switch frameType {
			case "error":
				ref, _ := raw["clientRef"].(string)
				if ref == clientRef {
					errMsg, _ := raw["message"].(string)
					events <- TurnEvent{Error: fmt.Errorf("mirasim error: %s", errMsg)}
					return
				}

			case "accepted":
				ref, _ := raw["clientRef"].(string)
				if ref == clientRef {
					sessionKey, _ = raw["sessionKey"].(string)
					if !subscribed && sessionKey != "" {
						subscribed = true
						_ = conn.WriteJSON(ClientSubscribeFrame{
							Type:       "subscribe",
							SessionKey: sessionKey,
						})
					}
				}

			case "session":
				sKey, _ := raw["sessionKey"].(string)
				if sKey != sessionKey {
					continue
				}

				var sessionFrame struct {
					Patch Patch `json:"patch"`
				}
				if err := json.Unmarshal(msg, &sessionFrame); err != nil {
					continue
				}

				patch := sessionFrame.Patch
				evt := TurnEvent{SessionKey: sessionKey}

				if patch.AppendText != "" {
					evt.AppendText = patch.AppendText
				}
				if patch.AppendReasoning != "" {
					evt.AppendReasoning = patch.AppendReasoning
				}

				if patch.Set != nil {
					if patch.Set.Usage != nil {
						evt.Usage = patch.Set.Usage
					}
					if patch.Set.Error != nil {
						evt.Error = fmt.Errorf("turn error: %v", patch.Set.Error)
						events <- evt
						return
					}
					if patch.Set.Phase == "done" {
						evt.Done = true
						events <- evt
						return
					}
				}

				if patch.Full != nil {
					if patch.Full.Usage != nil {
						evt.Usage = patch.Full.Usage
					}
					if patch.Full.Error != nil {
						evt.Error = fmt.Errorf("turn error: %v", patch.Full.Error)
						events <- evt
						return
					}
					if patch.Full.Phase == "done" {
						evt.Done = true
						events <- evt
						return
					}
				}

				if evt.AppendText != "" || evt.AppendReasoning != "" || evt.Usage != nil {
					events <- evt
				}

			case "snapshot":
				sKey, _ := raw["sessionKey"].(string)
				if sKey != sessionKey {
					continue
				}

				var snapshotFrame struct {
					Snapshot Snapshot `json:"snapshot"`
				}
				if err := json.Unmarshal(msg, &snapshotFrame); err == nil {
					snap := snapshotFrame.Snapshot
					if snap.Phase == "done" {
						events <- TurnEvent{
							SessionKey: sessionKey,
							Done:       true,
							Usage:      snap.Usage,
						}
						return
					}
				}
			}
		}
	}()

	return events, nil
}
