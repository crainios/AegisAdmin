package weblogs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"aegisadmin/backend/internal/protocol"
)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}
type Snapshot struct {
	Sources  []string
	Selected string
	Lines    []string
}
type Client struct{ backend Backend }

func New(backend Backend) *Client { return &Client{backend: backend} }

func (c *Client) LogSources(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reply, err := c.backend.Execute(protocol.Request{Domain: "logs", Command: "list"})
	if err != nil || !reply.Response.Success || reply.Response.Data == nil {
		return nil, errors.New("logs list unavailable")
	}
	var payload struct {
		Logs []struct {
			ID string `json:"id"`
		} `json:"logs"`
	}
	if err = decode(*reply.Response.Data, &payload); err != nil {
		return nil, fmt.Errorf("decode logs list: %w", err)
	}
	sources := make([]string, 0, len(payload.Logs))
	for _, item := range payload.Logs {
		if item.ID != "" {
			sources = append(sources, item.ID)
		}
	}
	return sources, nil
}

func (c *Client) LogsSnapshot(ctx context.Context, requested string) (Snapshot, error) {
	sources, err := c.LogSources(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{Sources: sources, Lines: []string{}}
	if requested != "" && slices.Contains(snapshot.Sources, requested) {
		snapshot.Selected = requested
	} else if len(snapshot.Sources) != 0 {
		snapshot.Selected = snapshot.Sources[0]
	}
	if snapshot.Selected == "" {
		return snapshot, nil
	}
	tail, err := c.backend.Execute(protocol.Request{Domain: "logs", Command: "tail", Arguments: []string{snapshot.Selected, "100"}})
	if err != nil || !tail.Response.Success || tail.Response.Data == nil {
		return Snapshot{}, errors.New("log content unavailable")
	}
	var content struct {
		Lines []string `json:"lines"`
	}
	if err = decode(*tail.Response.Data, &content); err != nil {
		return Snapshot{}, fmt.Errorf("decode log content: %w", err)
	}
	if content.Lines != nil {
		snapshot.Lines = content.Lines
		slices.Reverse(snapshot.Lines)
	}
	return snapshot, nil
}

func decode(data map[string]any, target any) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}
