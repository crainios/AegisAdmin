package webfirewall

import (
	"aegisadmin/backend/internal/protocol"
	"context"
	"encoding/json"
	"errors"
	"strconv"
)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}
type Info struct {
	Backend         string  `json:"backend"`
	Product         string  `json:"product"`
	Installed       bool    `json:"installed"`
	Version         *string `json:"version"`
	Active          bool    `json:"active"`
	IPv6            *bool   `json:"ipv6"`
	DefaultIncoming string  `json:"default_incoming"`
	DefaultOutgoing string  `json:"default_outgoing"`
	DefaultRouted   string  `json:"default_routed"`
}
type Rule struct {
	ID          int      `json:"id"`
	Action      string   `json:"action"`
	Direction   string   `json:"direction"`
	Protocol    string   `json:"protocol"`
	Ports       []string `json:"ports"`
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Family      string   `json:"family"`
}
type Snapshot struct {
	Info  Info
	Rules []Rule
}
type Client struct{ backend Backend }

func New(b Backend) *Client { return &Client{b} }
func (c *Client) FirewallSnapshot(ctx context.Context) (Snapshot, error) {
	if e := ctx.Err(); e != nil {
		return Snapshot{}, e
	}
	var s Snapshot
	d, e := c.call("info", nil)
	if e != nil {
		return s, e
	}
	if e = decode(d, &s.Info); e != nil {
		return s, e
	}
	d, e = c.call("rules", nil)
	if e != nil {
		return s, e
	}
	var p struct {
		Rules []Rule `json:"rules"`
	}
	if e = decode(d, &p); e != nil {
		return s, e
	}
	s.Rules = p.Rules
	if s.Rules == nil {
		s.Rules = []Rule{}
	}
	return s, nil
}
func (c *Client) Reload(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	_, e := c.call("reload", nil)
	return e
}
func (c *Client) SetEnabled(ctx context.Context, enabled bool) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	command := "disable"
	if enabled {
		command = "enable"
	}
	_, e := c.call(command, nil)
	return e
}
func (c *Client) Add(ctx context.Context, action, ports, protocol, source string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	_, e := c.call("add", []string{action, ports, protocol, source})
	return e
}
func (c *Client) Delete(ctx context.Context, id int) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if id < 1 {
		return errors.New("invalid rule")
	}
	d, e := c.call("delete-check", []string{strconv.Itoa(id)})
	if e != nil {
		return e
	}
	var check struct {
		RuleID      int    `json:"rule_id"`
		Fingerprint string `json:"fingerprint"`
	}
	if e = decode(d, &check); e != nil || check.RuleID != id || len(check.Fingerprint) != 64 {
		return errors.New("invalid delete check")
	}
	_, e = c.call("delete", []string{strconv.Itoa(id), check.Fingerprint, "yes"})
	return e
}
func (c *Client) call(command string, args []string) (map[string]any, error) {
	r, e := c.backend.Execute(protocol.Request{Domain: "firewall", Command: command, Arguments: args})
	if e != nil {
		return nil, e
	}
	if !r.Response.Success {
		if r.Response.Error != nil && r.Response.Error.Message != "" {
			return nil, errors.New(r.Response.Error.Message)
		}
		return nil, errors.New("L’action du pare-feu a échoué.")
	}
	if r.Response.Data == nil {
		return nil, errors.New("La réponse du pare-feu est incomplète.")
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
