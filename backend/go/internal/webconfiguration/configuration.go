package webconfiguration

import (
	"aegisadmin/backend/internal/protocol"
	"context"
	"errors"
)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}
type Client struct{ backend Backend }

func New(b Backend) *Client { return &Client{b} }
func (c *Client) Call(ctx context.Context, command string, args []string) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reply, err := c.backend.Execute(protocol.Request{Domain: "configuration", Command: command, Arguments: args})
	if err != nil || !reply.Response.Success || reply.Response.Data == nil {
		return nil, errors.New("configuration request failed")
	}
	return *reply.Response.Data, nil
}
