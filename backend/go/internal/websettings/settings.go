package websettings

import (
	"aegisadmin/backend/internal/protocol"
	"context"
	"encoding/json"
	"errors"
)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}
type AdminAccess struct {
	Available bool   `json:"available"`
	Enabled   bool   `json:"enabled"`
	Address   string `json:"address"`
	Port      int    `json:"port"`
	AllowFrom string `json:"allow_from"`
	URL       string `json:"url"`
	Message   string `json:"message"`
}
type Client struct{ backend Backend }

func New(backend Backend) *Client { return &Client{backend} }
func (c *Client) AdminAccess(ctx context.Context) (AdminAccess, error) {
	var result AdminAccess
	if err := ctx.Err(); err != nil {
		return result, err
	}
	data, err := c.call("status", nil)
	if err != nil {
		return result, err
	}
	encoded, _ := json.Marshal(data)
	err = json.Unmarshal(encoded, &result)
	return result, err
}
func (c *Client) UpdateAdminAccess(ctx context.Context, value AdminAccess) (AdminAccess, error) {
	var result AdminAccess
	if err := ctx.Err(); err != nil {
		return result, err
	}
	encoded, err := json.Marshal(map[string]any{"enabled": value.Enabled, "address": value.Address, "port": value.Port, "allow_from": value.AllowFrom})
	if err != nil {
		return result, err
	}
	data, err := c.call("update", []string{string(encoded)})
	if err != nil {
		return result, err
	}
	encoded, _ = json.Marshal(data)
	err = json.Unmarshal(encoded, &result)
	return result, err
}
func (c *Client) call(command string, args []string) (map[string]any, error) {
	reply, err := c.backend.Execute(protocol.Request{Domain: "admin-access", Command: command, Arguments: args})
	if err != nil || !reply.Response.Success || reply.Response.Data == nil {
		return nil, errors.New("admin access request failed")
	}
	return *reply.Response.Data, nil
}
