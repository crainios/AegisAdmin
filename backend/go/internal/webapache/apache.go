package webapache

import (
	"aegisadmin/backend/internal/protocol"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}
type VHost struct {
	ServerName   string  `json:"server_name"`
	Port         int     `json:"port"`
	ConfigFile   string  `json:"config_file"`
	DocumentRoot *string `json:"document_root"`
}
type Site struct {
	Filename    string   `json:"filename"`
	ConfigID    string   `json:"config_id"`
	Enabled     bool     `json:"enabled"`
	Size        int64    `json:"size_bytes"`
	ServerNames []string `json:"server_names"`
	Ports       []int    `json:"ports"`
}
type SiteConfig struct {
	Filename string `json:"filename"`
	ConfigID string `json:"config_id"`
	Enabled  bool   `json:"enabled"`
	Content  string `json:"content"`
}
type Module struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type Snapshot struct {
	Version, Built, ConfigMessage string
	ConfigValid                   bool
	VHosts                        []VHost
	Sites                         []Site
	Modules                       []Module
}
type Client struct{ backend Backend }

func New(b Backend) *Client { return &Client{b} }
func (c *Client) ApacheSnapshot(ctx context.Context) (Snapshot, error) {
	if e := ctx.Err(); e != nil {
		return Snapshot{}, e
	}
	r, e := c.backend.Execute(protocol.Request{Domain: "apache", Command: "overview"})
	if e != nil || !r.Response.Success || r.Response.Data == nil {
		return Snapshot{}, errors.New("apache overview unavailable")
	}
	var p struct {
		Info struct {
			Version string `json:"version"`
			Built   string `json:"built"`
		} `json:"info"`
		Config struct {
			Valid   bool   `json:"valid"`
			Message string `json:"message"`
		} `json:"configtest"`
		VHosts struct {
			Items []VHost `json:"virtual_hosts"`
		} `json:"vhosts"`
		Sites struct {
			Items []Site `json:"sites"`
		} `json:"sites"`
		Modules struct {
			Items []Module `json:"modules"`
		} `json:"modules"`
	}
	if e = decode(*r.Response.Data, &p); e != nil {
		return Snapshot{}, e
	}
	return Snapshot{Version: p.Info.Version, Built: p.Info.Built, ConfigValid: p.Config.Valid, ConfigMessage: p.Config.Message, VHosts: p.VHosts.Items, Sites: p.Sites.Items, Modules: p.Modules.Items}, nil
}
func (c *Client) ApacheAction(ctx context.Context, action string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if action != "reload" && action != "restart" {
		return errors.New("invalid apache action")
	}
	r, e := c.backend.Execute(protocol.Request{Domain: "apache", Command: action})
	if e != nil || !r.Response.Success {
		return errors.New("apache action failed")
	}
	return nil
}
func (c *Client) ApacheSite(ctx context.Context, identifier string) (SiteConfig, error) {
	var config SiteConfig
	if e := ctx.Err(); e != nil {
		return config, e
	}
	r, e := c.backend.Execute(protocol.Request{Domain: "apache", Command: "site", Arguments: []string{identifier}})
	if e != nil {
		return config, e
	}
	if !r.Response.Success || r.Response.Data == nil {
		return config, responseError(r)
	}
	if e = decode(*r.Response.Data, &config); e != nil {
		return config, e
	}
	return config, nil
}
func (c *Client) ApacheSiteAction(ctx context.Context, action, identifier, filename, content string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	var arguments []string
	switch action {
	case "create":
		payload, e := json.Marshal(map[string]string{"filename": filename, "content": base64.StdEncoding.EncodeToString([]byte(content))})
		if e != nil {
			return e
		}
		arguments = []string{string(payload)}
	case "update":
		payload, e := json.Marshal(map[string]string{"config_id": identifier, "content": base64.StdEncoding.EncodeToString([]byte(content))})
		if e != nil {
			return e
		}
		arguments = []string{string(payload)}
	case "enable", "disable", "delete":
		arguments = []string{identifier}
	default:
		return errors.New("invalid apache site action")
	}
	r, e := c.backend.Execute(protocol.Request{Domain: "apache", Command: action, Arguments: arguments})
	if e != nil {
		return e
	}
	if !r.Response.Success {
		return responseError(r)
	}
	return nil
}
func responseError(reply protocol.Reply) error {
	if reply.Response.Error != nil && reply.Response.Error.Message != "" {
		if output, ok := reply.Response.Error.Details["output"].(string); ok && output != "" {
			return fmt.Errorf("%s : %s", reply.Response.Error.Message, output)
		}
		return errors.New(reply.Response.Error.Message)
	}
	return errors.New("apache request failed")
}
func decode(d map[string]any, t any) error {
	b, e := json.Marshal(d)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, t)
}
