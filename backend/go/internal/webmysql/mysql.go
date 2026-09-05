package webmysql

import (
	"aegisadmin/backend/internal/protocol"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}
type Service struct {
	Unit    *string `json:"unit"`
	Service *string `json:"service"`
	Exists  bool    `json:"exists"`
	Active  bool    `json:"active"`
	Enabled bool    `json:"enabled"`
	State   string  `json:"state"`
}
type Server struct {
	Product              string  `json:"product"`
	Version              string  `json:"version"`
	VersionComment       string  `json:"version_comment"`
	Hostname             string  `json:"hostname"`
	Port                 int64   `json:"port"`
	Socket               string  `json:"socket"`
	DataDirectory        string  `json:"data_directory"`
	DefaultStorageEngine string  `json:"default_storage_engine"`
	Service              Service `json:"service"`
}
type Database struct {
	Name   string `json:"name"`
	Size   int64  `json:"size_bytes"`
	Tables int64  `json:"table_count"`
	Kind   string `json:"kind"`
}
type Snapshot struct {
	Server    Server
	Metrics   map[string]int64
	Databases []Database
	Warnings  []string
}
type Client struct{ backend Backend }

func New(b Backend) *Client { return &Client{b} }
func (c *Client) MySQLSnapshot(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	var s Snapshot
	serviceData, serviceErr := c.call("service")
	if serviceErr == nil {
		if e := decode(serviceData, &s.Server.Service); e != nil {
			return s, fmt.Errorf("decode mysql service: %w", e)
		}
		if !s.Server.Service.Exists {
			s.Server.Product = "MySQL / MariaDB"
			s.Metrics = map[string]int64{}
			s.Databases = []Database{}
			return s, nil
		}
	}
	info, e := c.call("info")
	if e != nil {
		if serviceErr == nil && s.Server.Service.Exists {
			s.Server.Product = detectedProduct(s.Server.Service)
			s.Warnings = append(s.Warnings, "Le service est détecté, mais la connexion locale à MySQL/MariaDB a été refusée. Vérifiez l’authentification du client système.")
			s.Metrics = map[string]int64{}
			s.Databases = []Database{}
			return s, nil
		}
		return s, e
	}
	if e = decode(info, &s.Server); e != nil {
		return s, fmt.Errorf("decode mysql info: %w", e)
	}
	status, e := c.call("status")
	if e != nil {
		s.Warnings = append(s.Warnings, "Les métriques MySQL ne sont pas disponibles.")
		s.Metrics = map[string]int64{}
	} else {
		var metrics struct {
			Metrics map[string]int64 `json:"metrics"`
		}
		if e = decode(status, &metrics); e != nil {
			return s, e
		}
		s.Metrics = metrics.Metrics
	}
	db, e := c.call("databases")
	if e != nil {
		s.Warnings = append(s.Warnings, "La liste des bases MySQL ne peut pas être consultée.")
		s.Databases = []Database{}
	} else {
		var databases struct {
			Databases []Database `json:"databases"`
		}
		if e = decode(db, &databases); e != nil {
			return s, e
		}
		s.Databases = databases.Databases
	}
	if s.Databases == nil {
		s.Databases = []Database{}
	}
	return s, nil
}

func detectedProduct(service Service) string {
	if service.Service != nil && strings.Contains(strings.ToLower(*service.Service), "maria") {
		return "MariaDB"
	}
	return "MySQL / MariaDB"
}
func (c *Client) RestartMySQL(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, e := c.call("restart")
	return e
}
func (c *Client) MySQLMetrics(ctx context.Context) (map[string]int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	status, err := c.call("status")
	if err != nil {
		return nil, err
	}
	var result struct {
		Metrics map[string]int64 `json:"metrics"`
	}
	if err := decode(status, &result); err != nil {
		return nil, err
	}
	return result.Metrics, nil
}
func (c *Client) call(command string) (map[string]any, error) {
	reply, e := c.backend.Execute(protocol.Request{Domain: "mysql", Command: command})
	if e != nil || !reply.Response.Success || reply.Response.Data == nil {
		return nil, errors.New("mysql backend request failed")
	}
	return *reply.Response.Data, nil
}
func decode(data map[string]any, target any) error {
	raw, e := json.Marshal(data)
	if e != nil {
		return e
	}
	return json.Unmarshal(raw, target)
}
