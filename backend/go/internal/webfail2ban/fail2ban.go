package webfail2ban

import (
	"aegisadmin/backend/internal/protocol"
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"regexp"
)

var jailPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}
type Info struct {
	Product         string `json:"product"`
	Version         string `json:"version"`
	ConfigDirectory string `json:"config_directory"`
}
type Status struct {
	Exists  bool     `json:"exists"`
	Active  bool     `json:"active"`
	Enabled bool     `json:"enabled"`
	State   string   `json:"state"`
	MainPID int64    `json:"main_pid"`
	Memory  int64    `json:"memory_bytes"`
	Tasks   int64    `json:"tasks"`
	Jails   []string `json:"jails"`
}
type Jail struct {
	CurrentlyFailed int64    `json:"currently_failed"`
	TotalFailed     int64    `json:"total_failed"`
	CurrentlyBanned int64    `json:"currently_banned"`
	TotalBanned     int64    `json:"total_banned"`
	BannedIPs       []string `json:"banned_ips"`
}
type Snapshot struct {
	Info                    Info
	Status                  Status
	ConfigValid             bool
	ConfigMessage, Selected string
	Jail                    *Jail
}
type Client struct{ backend Backend }

func New(b Backend) *Client { return &Client{b} }
func (c *Client) Fail2banSnapshot(ctx context.Context, requested string) (Snapshot, error) {
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
	d, e = c.call("status", nil)
	if e != nil {
		return s, e
	}
	if e = decode(d, &s.Status); e != nil {
		return s, e
	}
	d, e = c.call("configtest", nil)
	if e != nil {
		return s, e
	}
	var cfg struct {
		Valid   bool   `json:"valid"`
		Message string `json:"message"`
	}
	if e = decode(d, &cfg); e != nil {
		return s, e
	}
	s.ConfigValid = cfg.Valid
	s.ConfigMessage = cfg.Message
	if requested != "" {
		for _, j := range s.Status.Jails {
			if j == requested {
				s.Selected = j
			}
		}
	}
	if s.Selected == "" && len(s.Status.Jails) > 0 {
		s.Selected = s.Status.Jails[0]
	}
	if s.Selected != "" {
		d, e = c.call("jail", []string{s.Selected})
		if e != nil {
			return s, e
		}
		var payload struct {
			Jail   string `json:"jail"`
			Status Jail   `json:"status"`
		}
		if e = decode(d, &payload); e != nil {
			return s, e
		}
		if payload.Jail != s.Selected {
			return s, errors.New("fail2ban jail response mismatch")
		}
		s.Jail = &payload.Status
	}
	return s, nil
}
func (c *Client) Action(ctx context.Context, action, jail, address string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	args := []string{}
	switch action {
	case "reload", "restart":
	case "ban", "unban":
		if !jailPattern.MatchString(jail) {
			return errors.New("invalid jail")
		}
		ip, e := netip.ParseAddr(address)
		if e != nil {
			return errors.New("invalid address")
		}
		args = []string{jail, ip.String()}
	default:
		return errors.New("invalid action")
	}
	_, e := c.call(action, args)
	return e
}
func (c *Client) call(command string, args []string) (map[string]any, error) {
	r, e := c.backend.Execute(protocol.Request{Domain: "fail2ban", Command: command, Arguments: args})
	if e != nil || !r.Response.Success || r.Response.Data == nil {
		return nil, errors.New("fail2ban request failed")
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
