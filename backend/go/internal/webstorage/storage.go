package webstorage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"aegisadmin/backend/internal/protocol"
)

type Backend interface {
	Execute(protocol.Request) (protocol.Reply, error)
}

type Mount struct {
	ID             string `json:"id"`
	Source         string `json:"source"`
	Mount          string `json:"mount"`
	Filesystem     string `json:"filesystem"`
	PhysicalDevice string `json:"physical_device"`
	Size           int64  `json:"size"`
	Used           int64  `json:"used"`
	Available      int64  `json:"available"`
	Percent        int    `json:"percent"`
	Status         string
	StatusLabel    string
	SizeLabel      string
	UsedLabel      string
	AvailableLabel string
}

type Summary struct {
	Total, Normal, Warning, Danger int
}

type Snapshot struct {
	Mounts  []Mount
	Summary Summary
}

type Collector struct {
	backend Backend
}

func New(backend Backend) *Collector {
	return &Collector{backend: backend}
}

func (c *Collector) StorageSnapshot(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	reply, err := c.backend.Execute(protocol.Request{Domain: "storage", Command: "list"})
	if err != nil {
		return Snapshot{}, fmt.Errorf("read storage backend: %w", err)
	}
	if !reply.Response.Success || reply.Response.Data == nil {
		return Snapshot{}, errors.New("storage backend rejected request")
	}
	encoded, err := json.Marshal(*reply.Response.Data)
	if err != nil {
		return Snapshot{}, fmt.Errorf("encode storage response: %w", err)
	}
	var payload struct {
		Mounts []Mount `json:"mounts"`
	}
	if err = json.Unmarshal(encoded, &payload); err != nil {
		return Snapshot{}, fmt.Errorf("decode storage response: %w", err)
	}
	if payload.Mounts == nil {
		payload.Mounts = []Mount{}
	}
	snapshot := Snapshot{Mounts: payload.Mounts}
	for index := range snapshot.Mounts {
		mount := &snapshot.Mounts[index]
		if mount.Percent < 0 || mount.Percent > 100 || mount.Size < 0 || mount.Used < 0 || mount.Available < 0 {
			return Snapshot{}, errors.New("invalid storage values")
		}
		mount.Status, mount.StatusLabel = status(mount.Percent)
		mount.SizeLabel = formatBytes(mount.Size)
		mount.UsedLabel = formatBytes(mount.Used)
		mount.AvailableLabel = formatBytes(mount.Available)
		snapshot.Summary.Total++
		switch mount.Status {
		case "danger":
			snapshot.Summary.Danger++
		case "warning":
			snapshot.Summary.Warning++
		default:
			snapshot.Summary.Normal++
		}
	}
	return snapshot, nil
}

func status(percent int) (string, string) {
	if percent >= 90 {
		return "danger", "Critique"
	}
	if percent >= 80 {
		return "warning", "À surveiller"
	}
	return "success", "Normal"
}

func formatBytes(value int64) string {
	units := []string{"o", "Kio", "Mio", "Gio", "Tio", "Pio"}
	amount, unit := float64(value), 0
	for amount >= 1024 && unit < len(units)-1 {
		amount /= 1024
		unit++
	}
	decimals := 0
	if unit > 0 && amount < 10 {
		decimals = 1
	}
	return strings.ReplaceAll(strconv.FormatFloat(amount, 'f', decimals, 64), ".", ",") + " " + units[unit]
}
