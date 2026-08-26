package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"aegisadmin/backend/internal/protocol"
)

const (
	defaultTimeout     = 10 * time.Second
	startupRetryWindow = time.Second
	startupRetryDelay  = 25 * time.Millisecond
)

type Client struct {
	socketPath string
	timeout    time.Duration
	dial       func(string, string, time.Duration) (net.Conn, error)
	sleep      func(time.Duration)
}

func New(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		timeout:    defaultTimeout,
		dial:       net.DialTimeout,
		sleep:      time.Sleep,
	}
}

func (c *Client) Execute(request protocol.Request) (protocol.Reply, error) {
	connection, err := c.connect()
	if err != nil {
		return protocol.Reply{}, fmt.Errorf("connect to backend service: %w", err)
	}
	defer connection.Close()

	_ = connection.SetDeadline(time.Now().Add(c.timeout))

	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return protocol.Reply{}, fmt.Errorf("send backend request: %w", err)
	}

	var reply protocol.Reply
	decoder := json.NewDecoder(connection)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&reply); err != nil {
		return protocol.Reply{}, fmt.Errorf("read backend response: %w", err)
	}

	return reply, nil
}

func (c *Client) connect() (net.Conn, error) {
	deadline := time.Now().Add(startupRetryWindow)
	if timeoutDeadline := time.Now().Add(c.timeout); timeoutDeadline.Before(deadline) {
		deadline = timeoutDeadline
	}

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, os.ErrDeadlineExceeded
		}
		connection, err := c.dial("unix", c.socketPath, remaining)
		if err == nil {
			return connection, nil
		}
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ECONNREFUSED) {
			return nil, err
		}

		delay := startupRetryDelay
		if remaining < delay {
			delay = remaining
		}
		c.sleep(delay)
	}
}
