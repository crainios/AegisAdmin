package webtor

import (
	"aegisadmin/backend/internal/protocol"
	"context"
	"encoding/json"
	"errors"
)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}
type Info struct {
	Product    string `json:"product"`
	Version    string `json:"version"`
	ConfigFile string `json:"config_file"`
	Service    string `json:"service"`
	Unit       string `json:"unit"`
}
type Status struct {
	Exists      bool   `json:"exists"`
	Active      bool   `json:"active"`
	Enabled     bool   `json:"enabled"`
	LoadState   string `json:"load_state"`
	ActiveState string `json:"active_state"`
	State       string `json:"state"`
	MainPID     int64  `json:"main_pid"`
	Memory      int64  `json:"memory_bytes"`
	Tasks       int64  `json:"tasks"`
	Bootstrap   *int   `json:"bootstrap_percent"`
}
type Onion struct {
	ID       string           `json:"id"`
	Hostname *string          `json:"hostname"`
	Ports    []map[string]any `json:"ports"`
}
type Snapshot struct {
	Info                 Info
	Status               Status
	ConfigurationValid   bool
	ConfigurationMessage string
	Services             []Onion
}
type Client struct{ backend Backend }

func New(b Backend) *Client { return &Client{b} }
func (c *Client) TorSnapshot(ctx context.Context) (Snapshot, error) {
	if e := ctx.Err(); e != nil {
		return Snapshot{}, e
	}
	var s Snapshot
	d, e := c.call("info")
	if e != nil {
		return s, e
	}
	if e = decode(d, &s.Info); e != nil {
		return s, e
	}
	d, e = c.call("status")
	if e != nil {
		return s, e
	}
	if e = decode(d, &s.Status); e != nil {
		return s, e
	}
	d, e = c.call("configtest")
	if e != nil {
		return s, e
	}
	var config struct {
		Valid   bool   `json:"valid"`
		Message string `json:"message"`
	}
	if e = decode(d, &config); e != nil {
		return s, e
	}
	s.ConfigurationValid = config.Valid
	s.ConfigurationMessage = config.Message
	d, e = c.call("hidden-services")
	if e != nil {
		return s, e
	}
	var hidden struct {
		Services []Onion `json:"services"`
	}
	if e = decode(d, &hidden); e != nil {
		return s, e
	}
	s.Services = hidden.Services
	if s.Services == nil {
		s.Services = []Onion{}
	}
	return s, nil
}
func (c *Client) TorAction(ctx context.Context, action string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if action != "reload" && action != "restart" {
		return errors.New("invalid Tor action")
	}
	_, e := c.call(action)
	return e
}
func (c *Client) call(command string) (map[string]any, error) {
	r, e := c.backend.Execute(protocol.Request{Domain: "tor", Command: command})
	if e != nil || !r.Response.Success || r.Response.Data == nil {
		return nil, errors.New("Tor backend request failed")
	}
	return *r.Response.Data, nil
}
func decode(d map[string]any, t any) error {
	b, e := json.Marshal(d)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, t)
}
