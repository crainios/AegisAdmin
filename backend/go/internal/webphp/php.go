package webphp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"aegisadmin/backend/internal/protocol"
)

var runtimePattern = regexp.MustCompile(`^fpm-[0-9]+\.[0-9]+$`)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}
type CLI struct{ Version, SAPI, IniFile, ScanDir string }
type Instance struct {
	ID                  string `json:"id"`
	Version             string `json:"version"`
	Service             string `json:"service"`
	Exists              bool   `json:"exists"`
	Active              bool   `json:"active"`
	Enabled             bool   `json:"enabled"`
	State               string `json:"state"`
	Status, StatusLabel string
}
type Snapshot struct {
	CLI       CLI
	Instances []Instance
}
type Client struct{ backend Backend }

func New(backend Backend) *Client { return &Client{backend: backend} }

func (c *Client) PHPSnapshot(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	info, err := c.execute("info", nil)
	if err != nil {
		return Snapshot{}, err
	}
	var cli struct {
		Version string  `json:"version"`
		SAPI    string  `json:"sapi"`
		IniFile *string `json:"ini_file"`
		ScanDir *string `json:"scan_dir"`
	}
	if err = decode(info, &cli); err != nil {
		return Snapshot{}, fmt.Errorf("decode PHP info: %w", err)
	}
	snapshot := Snapshot{CLI: CLI{Version: cli.Version, SAPI: cli.SAPI, IniFile: optional(cli.IniFile), ScanDir: optional(cli.ScanDir)}}
	fpm, err := c.execute("fpm", nil)
	if err != nil {
		return Snapshot{}, err
	}
	var payload struct {
		Instances []Instance `json:"instances"`
	}
	if err = decode(fpm, &payload); err != nil {
		return Snapshot{}, fmt.Errorf("decode PHP-FPM: %w", err)
	}
	if payload.Instances == nil {
		payload.Instances = []Instance{}
	}
	for index := range payload.Instances {
		instance := &payload.Instances[index]
		if !runtimePattern.MatchString(instance.ID) {
			return Snapshot{}, errors.New("invalid PHP runtime")
		}
		instance.Status, instance.StatusLabel = status(*instance)
	}
	snapshot.Instances = payload.Instances
	return snapshot, nil
}

func (c *Client) RestartPHP(ctx context.Context, runtime string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !runtimePattern.MatchString(runtime) {
		return errors.New("invalid PHP runtime")
	}
	_, err := c.execute("restart", []string{runtime})
	return err
}

func (c *Client) execute(command string, arguments []string) (map[string]any, error) {
	reply, err := c.backend.Execute(protocol.Request{Domain: "php", Command: command, Arguments: arguments})
	if err != nil || !reply.Response.Success || reply.Response.Data == nil {
		return nil, errors.New("PHP backend request failed")
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
func optional(value *string) string {
	if value == nil || *value == "" {
		return "Non disponible"
	}
	return *value
}
func status(instance Instance) (string, string) {
	if !instance.Exists {
		return "neutral", "Non installée"
	}
	if instance.Active {
		return "success", "Active"
	}
	if instance.State == "failed" {
		return "danger", "En échec"
	}
	return "warning", "Arrêtée"
}
