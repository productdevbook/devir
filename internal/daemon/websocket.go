package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// DefaultWSPort is the default WebSocket server port
	DefaultWSPort = 9222

	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

// WSServer handles WebSocket connections for browser clients
type WSServer struct {
	daemon   *Daemon
	upgrader websocket.Upgrader
	clients  map[*wsClient]bool
	mu       sync.RWMutex
	server   *http.Server
	stopCh   chan struct{}
}

type wsClient struct {
	conn   *websocket.Conn
	sendCh chan []byte
	server *WSServer
}

// WSLogMessage is the JSON message sent to WebSocket clients
type WSLogMessage struct {
	Type    string    `json:"type"`
	Time    time.Time `json:"time"`
	Service string    `json:"service"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// WSStatusMessage is the JSON message for service status
type WSStatusMessage struct {
	Type     string            `json:"type"`
	Services []WSServiceStatus `json:"services"`
}

// WSServiceStatus represents a service status for WebSocket
type WSServiceStatus struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Status  string `json:"status"` // running, stopped, completed, failed, waiting
	Port    int    `json:"port,omitempty"`
	Color   string `json:"color"`
	Icon    string `json:"icon,omitempty"`
	Type    string `json:"type,omitempty"` // service, oneshot, interval, http
}

// WSCommand is an incoming command from WebSocket client
type WSCommand struct {
	Action  string `json:"action"`  // restart, stop, start, clear
	Service string `json:"service"` // service name (optional for some actions)
}

// WSResponse is a response to a command
type WSResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// NewWSServer creates a new WebSocket server
func NewWSServer(daemon *Daemon) *WSServer {
	return &WSServer{
		daemon:  daemon,
		clients: make(map[*wsClient]bool),
		stopCh:  make(chan struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// Allow connections from localhost only
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				// Allow chrome-extension:// and localhost origins
				return origin == "chrome-extension://" ||
					origin == "http://localhost" ||
					origin == "https://localhost" ||
					len(origin) > 17 && origin[:17] == "chrome-extension:" ||
					len(origin) > 16 && origin[:16] == "http://localhost" ||
					len(origin) > 17 && origin[:17] == "https://localhost"
			},
		},
	}
}

// Start starts the WebSocket server on the specified port
func (ws *WSServer) Start(port int) error {
	if port <= 0 {
		port = DefaultWSPort
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/logs", ws.handleLogs)
	mux.HandleFunc("/status", ws.handleStatus)

	ws.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: mux,
	}

	go func() {
		if err := ws.server.ListenAndServe(); err != http.ErrServerClosed {
			// Log error but don't crash - WebSocket is optional
			fmt.Printf("WebSocket server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the WebSocket server
func (ws *WSServer) Stop() {
	close(ws.stopCh)

	ws.mu.Lock()
	for client := range ws.clients {
		_ = client.conn.Close()
	}
	ws.mu.Unlock()

	if ws.server != nil {
		_ = ws.server.Close()
	}
}

func (ws *WSServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &wsClient{
		conn:   conn,
		sendCh: make(chan []byte, 256),
		server: ws,
	}

	ws.mu.Lock()
	ws.clients[client] = true
	ws.mu.Unlock()

	go client.writePump()
	go client.readPump()
}

func (ws *WSServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// Send current status and close
	var statuses []WSServiceStatus

	if ws.daemon.runner != nil {
		for name, state := range ws.daemon.runner.Services {
			state.Mu.Lock()
			s := WSServiceStatus{
				Name:    name,
				Running: state.Running,
				Port:    state.Service.Port,
				Color:   state.Service.Color,
				Icon:    state.Service.Icon,
			}
			state.Mu.Unlock()
			statuses = append(statuses, s)
		}
	}

	msg := WSStatusMessage{
		Type:     "status",
		Services: statuses,
	}

	data, _ := json.Marshal(msg)
	_ = conn.WriteMessage(websocket.TextMessage, data)
	_ = conn.Close()
}

// BroadcastLog sends a log entry to all connected WebSocket clients
func (ws *WSServer) BroadcastLog(entry LogEntryData) {
	msg := WSLogMessage{
		Type:    "log",
		Time:    entry.Time,
		Service: entry.Service,
		Level:   entry.Level,
		Message: entry.Message,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	ws.mu.RLock()
	defer ws.mu.RUnlock()

	for client := range ws.clients {
		select {
		case client.sendCh <- data:
		default:
			// Drop if buffer full
		}
	}
}

func (c *wsClient) readPump() {
	defer func() {
		c.server.mu.Lock()
		delete(c.server.clients, c)
		c.server.mu.Unlock()
		close(c.sendCh)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		// Handle incoming commands
		var cmd WSCommand
		if err := json.Unmarshal(message, &cmd); err == nil && cmd.Action != "" {
			c.server.handleCommand(c, cmd)
		}
	}
}

func (ws *WSServer) handleCommand(c *wsClient, cmd WSCommand) {
	var resp WSResponse
	resp.Type = "response"

	switch cmd.Action {
	case "restart":
		if cmd.Service == "" {
			resp.Error = "service name required"
		} else if ws.daemon.runner == nil {
			resp.Error = "no services running"
		} else if _, ok := ws.daemon.runner.Services[cmd.Service]; !ok {
			resp.Error = "unknown service: " + cmd.Service
		} else {
			ws.daemon.runner.RestartService(cmd.Service)
			resp.Success = true
			resp.Message = "restarting " + cmd.Service
		}

	case "stop":
		if cmd.Service != "" {
			// Stop specific service
			if ws.daemon.runner == nil {
				resp.Error = "no services running"
			} else if _, ok := ws.daemon.runner.Services[cmd.Service]; !ok {
				resp.Error = "unknown service: " + cmd.Service
			} else {
				ws.daemon.runner.StopService(cmd.Service)
				resp.Success = true
				resp.Message = "stopped " + cmd.Service
			}
		} else {
			// Stop all services
			if ws.daemon.runner != nil {
				ws.daemon.runner.Stop()
				resp.Success = true
				resp.Message = "stopping all services"
			} else {
				resp.Error = "no services running"
			}
		}

	case "start":
		if cmd.Service == "" {
			resp.Error = "service name required"
		} else if ws.daemon.runner == nil {
			resp.Error = "no services running"
		} else if state, ok := ws.daemon.runner.Services[cmd.Service]; !ok {
			resp.Error = "unknown service: " + cmd.Service
		} else {
			state.Mu.Lock()
			isRunning := state.Running
			state.Mu.Unlock()

			if isRunning {
				resp.Error = "service already running"
			} else {
				ws.daemon.runner.StartService(cmd.Service)
				resp.Success = true
				resp.Message = "starting " + cmd.Service
			}
		}

	case "clear":
		if ws.daemon.runner != nil {
			ws.daemon.runner.ClearLogs(cmd.Service)
			resp.Success = true
			resp.Message = "logs cleared"
		}

	case "status":
		ws.sendStatus(c)
		return

	default:
		resp.Error = "unknown action: " + cmd.Action
	}

	data, _ := json.Marshal(resp)
	c.sendCh <- data
}

func (ws *WSServer) sendStatus(c *wsClient) {
	var statuses []WSServiceStatus

	if ws.daemon.runner != nil {
		for name, state := range ws.daemon.runner.Services {
			state.Mu.Lock()
			s := WSServiceStatus{
				Name:    name,
				Running: state.Running,
				Status:  string(state.Status),
				Port:    state.Service.Port,
				Color:   state.Service.Color,
				Icon:    state.Service.Icon,
				Type:    string(state.Service.GetEffectiveType()),
			}
			state.Mu.Unlock()
			statuses = append(statuses, s)
		}
	}

	msg := WSStatusMessage{
		Type:     "status",
		Services: statuses,
	}

	data, _ := json.Marshal(msg)
	c.sendCh <- data
}

func (c *wsClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.sendCh:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
