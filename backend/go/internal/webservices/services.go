package webservices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"aegisadmin/backend/internal/protocol"
)

var identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9@._-]*$`)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}

type Service struct {
	ID          string `json:"id"`
	Exists      bool   `json:"exists"`
	Active      bool   `json:"active"`
	Enabled     bool   `json:"enabled"`
	State       string `json:"state"`
	Status      string
	StatusLabel string
}

type Summary struct {
	Total, Installed, Active, Inactive, Missing int
}

type Snapshot struct {
	Services []Service
	Summary  Summary
}

type Client struct{ backend Backend }

func New(backend Backend) *Client { return &Client{backend: backend} }

func (c *Client) ServicesSnapshot(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	reply, err := c.backend.Execute(protocol.Request{Domain: "services", Command: "list"})
	if err != nil {
		return Snapshot{}, fmt.Errorf("read services backend: %w", err)
	}
	if !reply.Response.Success || reply.Response.Data == nil {
		return Snapshot{}, errors.New("services backend rejected request")
	}
	encoded, err := json.Marshal(*reply.Response.Data)
	if err != nil {
		return Snapshot{}, fmt.Errorf("encode services response: %w", err)
	}
	var payload struct {
		Services []Service `json:"services"`
	}
	if err = json.Unmarshal(encoded, &payload); err != nil {
		return Snapshot{}, fmt.Errorf("decode services response: %w", err)
	}
	if payload.Services == nil {
		payload.Services = []Service{}
	}
	snapshot := Snapshot{Services: payload.Services}
	for index := range snapshot.Services {
		service := &snapshot.Services[index]
		if !identifierPattern.MatchString(service.ID) {
			return Snapshot{}, errors.New("invalid service identifier")
		}
		service.Status, service.StatusLabel = serviceStatus(*service)
		snapshot.Summary.Total++
		if !service.Exists {
			snapshot.Summary.Missing++
			continue
		}
		snapshot.Summary.Installed++
		if service.Active {
			snapshot.Summary.Active++
		} else {
			snapshot.Summary.Inactive++
		}
	}
	return snapshot, nil
}

func (c *Client) RestartService(ctx context.Context, service string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !identifierPattern.MatchString(service) {
		return errors.New("invalid service identifier")
	}
	reply, err := c.backend.Execute(protocol.Request{Domain: "services", Command: "restart", Arguments: []string{service}})
	if err != nil {
		return fmt.Errorf("restart service backend: %w", err)
	}
	if !reply.Response.Success {
		return errors.New("services backend rejected restart")
	}
	return nil
}

func serviceStatus(service Service) (string, string) {
	if !service.Exists {
		return "neutral", "Non installé"
	}
	if service.Active {
		return "success", "Actif"
	}
	if service.State == "failed" {
		return "danger", "En échec"
	}
	return "warning", "Arrêté"
}
