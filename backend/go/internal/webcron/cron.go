package webcron

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
	Product          string `json:"product"`
	Version          string `json:"version"`
	Service          string `json:"service"`
	Unit             string `json:"unit"`
	AnacronAvailable bool   `json:"anacron_available"`
}
type Status struct {
	Exists      bool   `json:"exists"`
	Active      bool   `json:"active"`
	Enabled     bool   `json:"enabled"`
	LoadState   string `json:"load_state"`
	ActiveState string `json:"active_state"`
	State       string `json:"state"`
	MainPID     int64  `json:"main_pid"`
	MemoryBytes int64  `json:"memory_bytes"`
	Tasks       int64  `json:"tasks"`
}
type User struct {
	Name   string `json:"name"`
	System bool   `json:"system"`
}
type Job struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Type     string `json:"type"`
	User     string `json:"user"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Line     *int   `json:"line"`
	Enabled  bool   `json:"enabled"`
	Editable bool   `json:"editable"`
	Managed  bool   `json:"managed"`
}
type Snapshot struct {
	Info   Info
	Status Status
	Users  []User
	Jobs   []Job
}
type ExecutionResult struct {
	ExecutionID string `json:"execution_id"`
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

func (c *Client) CronSnapshot(ctx context.Context) (Snapshot, error) {
	var result Snapshot
	if err := ctx.Err(); err != nil {
		return result, err
	}
	for _, call := range []struct {
		command string
		target  any
	}{
		{"info", &result.Info}, {"status", &result.Status},
	} {
		data, err := c.call(call.command, nil)
		if err != nil {
			return result, err
		}
		if err := decode(data, call.target); err != nil {
			return result, err
		}
	}
	data, err := c.call("users", nil)
	if err != nil {
		return result, err
	}
	var users struct {
		Users []User `json:"users"`
	}
	if err := decode(data, &users); err != nil {
		return result, err
	}
	result.Users = users.Users
	data, err = c.call("jobs", nil)
	if err != nil {
		return result, err
	}
	var jobs struct {
		Jobs []Job `json:"jobs"`
	}
	if err := decode(data, &jobs); err != nil {
		return result, err
	}
	result.Jobs = jobs.Jobs
	if result.Users == nil {
		result.Users = []User{}
	}
	if result.Jobs == nil {
		result.Jobs = []Job{}
	}
	return result, nil
}

func (c *Client) CronAction(ctx context.Context, action, user, id, schedule, command string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var arguments []string
	switch action {
	case "create":
		arguments = []string{user, schedule, command}
	case "update":
		arguments = []string{user, id, schedule, command}
	case "suspend", "resume", "delete", "run":
		arguments = []string{user, id}
	default:
		return "", errors.New("invalid cron action")
	}
	data, err := c.call(action, arguments)
	if err != nil {
		return "", err
	}
	if action != "run" {
		return "", nil
	}
	value, ok := data["execution_id"].(string)
	if !ok || len(value) != 32 {
		return "", errors.New("invalid cron execution")
	}
	return value, nil
}

func (c *Client) CronResult(ctx context.Context, id string) (ExecutionResult, error) {
	var result ExecutionResult
	if err := ctx.Err(); err != nil {
		return result, err
	}
	data, err := c.call("run-result", []string{id})
	if err != nil {
		return result, err
	}
	err = decode(data, &result)
	return result, err
}

func (c *Client) call(command string, arguments []string) (map[string]any, error) {
	reply, err := c.backend.Execute(protocol.Request{Domain: "cron", Command: command, Arguments: arguments})
	if err != nil {
		return nil, err
	}
	if !reply.Response.Success {
		if reply.Response.Error != nil {
			return nil, errors.New(reply.Response.Error.Message)
		}
		return nil, errors.New("cron request failed")
	}
	if reply.Response.Data == nil {
		return nil, errors.New("cron response is empty")
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
