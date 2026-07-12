package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// WebSocket connection settings
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024 // 512KB

	// Reconnect settings
	initialBackoff = 1 * time.Second
	maxBackoff     = 5 * time.Minute
	backoffFactor  = 2.0
)

type WSClient struct {
	url          string
	conn         *websocket.Conn
	sendCh       chan []byte
	done         chan struct{}
	gatewayID    string
	apiKey       string
	onMessage    func([]byte)
	reconnecting bool
}

type RegisterMessage struct {
	Type      string `json:"type"`
	GatewayID string `json:"gateway_id"`
	APIKey    string `json:"api_key,omitempty"`
}

type CommandMessage struct {
	Type      string `json:"type"`
	CommandID string `json:"command_id"`
	NodeIP    string `json:"node_ip"`
	Command   string `json:"command"`
}

type ResponseMessage struct {
	Type      string `json:"type"`
	CommandID string `json:"command_id"`
	Success   bool   `json:"success"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

func NewWSClient(url, gatewayID, apiKey string, onMessage func([]byte)) *WSClient {
	return &WSClient{
		url:       url,
		gatewayID: gatewayID,
		apiKey:    apiKey,
		sendCh:    make(chan []byte, 256),
		done:      make(chan struct{}),
		onMessage: onMessage,
	}
}

// Connect establishes WebSocket connection with exponential backoff
func (c *WSClient) Connect() error {
	backoff := initialBackoff

	for {
		log.Printf("Attempting to connect to %s...", c.url)

		conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
		if err != nil {
			log.Printf("Connection failed: %v. Retrying in %v...", err, backoff)
			time.Sleep(backoff)
			backoff = time.Duration(math.Min(float64(backoff)*backoffFactor, float64(maxBackoff)))
			continue
		}

		c.conn = conn
		log.Println("WebSocket connected successfully")

		// Register this gateway
		if err := c.register(); err != nil {
			log.Printf("Registration failed: %v", err)
			conn.Close()
			time.Sleep(backoff)
			backoff = time.Duration(math.Min(float64(backoff)*backoffFactor, float64(maxBackoff)))
			continue
		}

		log.Println("Gateway registered successfully")
		return nil
	}
}

// register sends registration message to the server
func (c *WSClient) register() error {
	msg := RegisterMessage{
		Type:      "register",
		GatewayID: c.gatewayID,
		APIKey:    c.apiKey,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal register message: %w", err)
	}

	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("failed to send register message: %w", err)
	}

	return nil
}

// Run starts the read and write loops
func (c *WSClient) Run() {
	// Start write pump
	go c.writePump()

	// Start read pump (blocks here)
	c.readPump()
}

// readPump handles incoming messages and triggers reconnect on error
func (c *WSClient) readPump() {
	defer func() {
		c.conn.Close()
		if !c.reconnecting {
			log.Println("Connection closed. Attempting to reconnect...")
			c.reconnecting = true
			go c.reconnect()
		}
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	c.conn.SetReadLimit(maxMessageSize)

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			return
		}

		log.Printf("Received message: %s", string(message))

		// Pass message to handler
		if c.onMessage != nil {
			go c.onMessage(message)
		}
	}
}

// writePump sends messages from the send channel and handles ping/pong
func (c *WSClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.sendCh:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Write error: %v", err)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Ping error: %v", err)
				return
			}

		case <-c.done:
			return
		}
	}
}

// Send queues a message to be sent
func (c *WSClient) Send(data []byte) {
	select {
	case c.sendCh <- data:
	default:
		log.Println("Send channel full, dropping message")
	}
}

// reconnect attempts to reconnect with exponential backoff
func (c *WSClient) reconnect() {
	backoff := initialBackoff

	for {
		select {
		case <-c.done:
			return
		case <-time.After(backoff):
			log.Printf("Reconnecting to %s...", c.url)

			conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
			if err != nil {
				log.Printf("Reconnection failed: %v. Retrying in %v...", err, backoff)
				backoff = time.Duration(math.Min(float64(backoff)*backoffFactor, float64(maxBackoff)))
				continue
			}

			c.conn = conn
			log.Println("Reconnected successfully")

			// Register again
			if err := c.register(); err != nil {
				log.Printf("Re-registration failed: %v", err)
				conn.Close()
				backoff = time.Duration(math.Min(float64(backoff)*backoffFactor, float64(maxBackoff)))
				continue
			}

			log.Println("Re-registered successfully")

			// Reset backoff and reconnecting flag
			backoff = initialBackoff
			c.reconnecting = false

			// Restart pumps
			go c.writePump()
			c.readPump()
			return
		}
	}
}

// Close gracefully shuts down the WebSocket connection
func (c *WSClient) Close() {
	close(c.done)
	if c.conn != nil {
		c.conn.Close()
	}
}
