package webcertbot

import (
	"context"
	"encoding/json"
	"errors"

	"aegisadmin/backend/internal/protocol"
)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}

type Info struct {
	Installed                                  bool `json:"installed"`
	Product, Version, Executable, Installation string
	Package                                    *string  `json:"package"`
	PackageVersion                             *string  `json:"package_version"`
	Plugins                                    []string `json:"plugins"`
}
type Status struct {
	Service       string  `json:"service"`
	Unit          string  `json:"unit"`
	Timer         string  `json:"timer"`
	Exists        bool    `json:"exists"`
	TimerExists   bool    `json:"timer_exists"`
	TimerActive   bool    `json:"timer_active"`
	TimerEnabled  bool    `json:"timer_enabled"`
	ServiceActive bool    `json:"service_active"`
	ServiceState  string  `json:"service_state"`
	LastResult    string  `json:"last_result"`
	LastExitCode  int     `json:"last_exit_code"`
	LastTrigger   *string `json:"last_trigger"`
	NextTrigger   *string `json:"next_trigger"`
}
type Certificate struct {
	Name          string   `json:"name"`
	Serial        string   `json:"serial"`
	KeyType       string   `json:"key_type"`
	Domains       []string `json:"domains"`
	Issuer        string   `json:"issuer"`
	Expiry        string   `json:"expiry"`
	DaysRemaining int      `json:"days_remaining"`
	Valid         bool     `json:"valid"`
}
type Snapshot struct {
	Info         Info
	Status       Status
	Certificates []Certificate
}
type ActionResult struct {
	ExecutionID string `json:"execution_id"`
	Action      string `json:"action"`
	Status      string `json:"status"`
	ExitCode    *int   `json:"exit_code"`
	TimedOut    bool   `json:"timed_out"`
	Truncated   bool   `json:"truncated"`
	DurationMS  int64  `json:"duration_ms"`
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
}
type Client struct{ backend Backend }

func New(backend Backend) *Client { return &Client{backend: backend} }

func (c *Client) Snapshot(ctx context.Context) (Snapshot, error) {
	var result Snapshot
	if err := ctx.Err(); err != nil {
		return result, err
	}
	for _, call := range []struct {
		command string
		target  any
	}{{"info", &result.Info}, {"status", &result.Status}} {
		data, err := c.call(call.command, nil)
		if err != nil {
			return result, err
		}
		if err := decode(data, call.target); err != nil {
			return result, err
		}
	}
	data, err := c.call("certificates", nil)
	if err != nil {
		return result, err
	}
	var list struct {
		Certificates []Certificate `json:"certificates"`
	}
	if err := decode(data, &list); err != nil {
		return result, err
	}
	result.Certificates = list.Certificates
	if result.Certificates == nil {
		result.Certificates = []Certificate{}
	}
	return result, nil
}
func (c *Client) Start(ctx context.Context, action string, arguments []string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	switch action {
	case "renew-test", "renew", "issue", "delete", "reinstall", "renew-replace":
	default:
		return "", errors.New("invalid certbot action")
	}
	data, err := c.call(action, arguments)
	if err != nil {
		return "", err
	}
	id, ok := data["execution_id"].(string)
	if !ok || len(id) != 32 {
		return "", errors.New("invalid certbot execution")
	}
	return id, nil
}
func (c *Client) Result(ctx context.Context, id string) (ActionResult, error) {
	var result ActionResult
	if err := ctx.Err(); err != nil {
		return result, err
	}
	data, err := c.call("action-result", []string{id})
	if err != nil {
		return result, err
	}
	err = decode(data, &result)
	return result, err
}
func (c *Client) call(command string, arguments []string) (map[string]any, error) {
	reply, err := c.backend.Execute(protocol.Request{Domain: "certbot", Command: command, Arguments: arguments})
	if err != nil {
		return nil, err
	}
	if !reply.Response.Success {
		if reply.Response.Error != nil && reply.Response.Error.Message != "" {
			return nil, errors.New(reply.Response.Error.Message)
		}
		return nil, errors.New("L’action Certbot a échoué.")
	}
	if reply.Response.Data == nil {
		return nil, errors.New("La réponse Certbot est incomplète.")
	}
	return *reply.Response.Data, nil
}
func decode(data map[string]any, target any) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}
