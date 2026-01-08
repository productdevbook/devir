package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// Client connects to a daemon
type Client struct {
	conn     net.Conn
	sendCh   chan Message
	recvCh   chan Message
	closeCh  chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	closed   bool
	handlers map[string]func(Message)
	handlerMu sync.RWMutex
}

// Connect connects to an existing daemon
func Connect(socketPath string) (*Client, error) {
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}

	c := &Client{
		conn:     conn,
		sendCh:   make(chan Message, 100),
		recvCh:   make(chan Message, 100),
		closeCh:  make(chan struct{}),
		handlers: make(map[string]func(Message)),
	}

	c.wg.Add(2)
	go c.readLoop()
	go c.writeLoop()

	return c, nil
}

func (c *Client) readLoop() {
	defer c.wg.Done()

	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		// Check for registered handler
		c.handlerMu.RLock()
		handler, ok := c.handlers[msg.Type]
		c.handlerMu.RUnlock()

		if ok {
			handler(msg)
		} else {
			select {
			case c.recvCh <- msg:
			default:
				// Drop if buffer full
			}
		}
	}
}

func (c *Client) writeLoop() {
	defer c.wg.Done()

	encoder := json.NewEncoder(c.conn)
	for {
		select {
		case <-c.closeCh:
			return
		case msg := <-c.sendCh:
			if err := encoder.Encode(msg); err != nil {
				return
			}
		}
	}
}

// OnMessage registers a handler for a message type
func (c *Client) OnMessage(msgType string, handler func(Message)) {
	c.handlerMu.Lock()
	c.handlers[msgType] = handler
	c.handlerMu.Unlock()
}

// Send sends a message to the daemon
func (c *Client) Send(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("client closed")
	}

	select {
	case c.sendCh <- msg:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

// Receive returns the receive channel for messages
func (c *Client) Receive() <-chan Message {
	return c.recvCh
}

// Close closes the client connection
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	close(c.closeCh)
	c.conn.Close()
	c.wg.Wait()
	return nil
}

// --- Convenience methods ---

// Start sends a start request
func (c *Client) Start(services []string, killPorts bool) error {
	msg, err := NewMessage(MsgStart, StartRequest{
		Services:  services,
		KillPorts: killPorts,
	})
	if err != nil {
		return err
	}
	return c.Send(msg)
}

// Stop sends a stop request
func (c *Client) Stop() error {
	msg, _ := NewMessage(MsgStop, struct{}{})
	return c.Send(msg)
}

// Restart sends a restart request
func (c *Client) Restart(service string) error {
	msg, err := NewMessage(MsgRestart, RestartRequest{Service: service})
	if err != nil {
		return err
	}
	return c.Send(msg)
}

// Status sends a status request
func (c *Client) Status() error {
	msg, _ := NewMessage(MsgStatus, struct{}{})
	return c.Send(msg)
}

// Logs sends a logs request
func (c *Client) Logs(service string, lines int) error {
	msg, err := NewMessage(MsgLogs, LogsRequest{Service: service, Lines: lines})
	if err != nil {
		return err
	}
	return c.Send(msg)
}

// CheckPorts sends a check ports request
func (c *Client) CheckPorts() error {
	msg, _ := NewMessage(MsgCheckPorts, struct{}{})
	return c.Send(msg)
}

// KillPorts sends a kill ports request
func (c *Client) KillPorts(ports []int) error {
	msg, err := NewMessage(MsgKillPorts, KillPortsRequest{Ports: ports})
	if err != nil {
		return err
	}
	return c.Send(msg)
}

// WaitForResponse waits for a specific response type
func (c *Client) WaitForResponse(msgType string, timeout time.Duration) (Message, error) {
	deadline := time.Now().Add(timeout)

	for {
		select {
		case msg := <-c.recvCh:
			if msg.Type == msgType {
				return msg, nil
			}
			if msg.Type == MsgError {
				errResp, _ := ParsePayload[ErrorResponse](msg)
				return msg, fmt.Errorf("daemon error: %s", errResp.Error)
			}
		case <-time.After(time.Until(deadline)):
			return Message{}, fmt.Errorf("timeout waiting for %s", msgType)
		}
	}
}

// StartAndWait starts services and waits for confirmation
func (c *Client) StartAndWait(services []string, killPorts bool, timeout time.Duration) ([]string, error) {
	if err := c.Start(services, killPorts); err != nil {
		return nil, err
	}

	msg, err := c.WaitForResponse(MsgStarted, timeout)
	if err != nil {
		return nil, err
	}

	resp, err := ParsePayload[StartedResponse](msg)
	if err != nil {
		return nil, err
	}

	return resp.Services, nil
}

// StatusSync gets status synchronously
func (c *Client) StatusSync(timeout time.Duration) ([]ServiceStatus, error) {
	if err := c.Status(); err != nil {
		return nil, err
	}

	msg, err := c.WaitForResponse(MsgStatusResponse, timeout)
	if err != nil {
		return nil, err
	}

	resp, err := ParsePayload[StatusResponse](msg)
	if err != nil {
		return nil, err
	}

	return resp.Services, nil
}

// LogsSync gets logs synchronously
func (c *Client) LogsSync(service string, lines int, timeout time.Duration) ([]LogEntryData, error) {
	if err := c.Logs(service, lines); err != nil {
		return nil, err
	}

	msg, err := c.WaitForResponse(MsgLogsResponse, timeout)
	if err != nil {
		return nil, err
	}

	resp, err := ParsePayload[LogsResponse](msg)
	if err != nil {
		return nil, err
	}

	return resp.Logs, nil
}

// CheckPortsSync checks ports synchronously
func (c *Client) CheckPortsSync(timeout time.Duration) (PortsResponse, error) {
	if err := c.CheckPorts(); err != nil {
		return PortsResponse{}, err
	}

	msg, err := c.WaitForResponse(MsgPortsResponse, timeout)
	if err != nil {
		return PortsResponse{}, err
	}

	return ParsePayload[PortsResponse](msg)
}

// KillPortsSync kills ports synchronously
func (c *Client) KillPortsSync(ports []int, timeout time.Duration) (KillPortsResponse, error) {
	if err := c.KillPorts(ports); err != nil {
		return KillPortsResponse{}, err
	}

	msg, err := c.WaitForResponse(MsgKillResponse, timeout)
	if err != nil {
		return KillPortsResponse{}, err
	}

	return ParsePayload[KillPortsResponse](msg)
}
