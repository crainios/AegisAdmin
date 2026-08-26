package webdashboard

import (
	"context"
	"errors"
	"testing"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

func TestSnapshotAggregatesBackendDomains(t *testing.T) {
	backend := fakeBackend{data: map[string]map[string]any{
		"system": {
			"host":        map[string]any{"hostname": "srv1", "operating_system": "Linux", "distribution": "Debian", "version": "13", "pretty_name": "Debian 13", "kernel": "6.12", "architecture": "amd64", "uptime_seconds": float64(90061)},
			"cpu":         map[string]any{"usage_percent": 12.5, "cores": float64(4), "model": "Test CPU", "load_average": []any{0.1, 0.2, 0.3}},
			"memory":      map[string]any{"percent": 50.0, "used_bytes": float64(4 << 30), "available_bytes": float64(4 << 30), "total_bytes": float64(8 << 30)},
			"temperature": map[string]any{"available": true, "celsius": 52.4, "sensor": "Package"},
			"processes":   map[string]any{"total": float64(1234)},
		},
		"storage": {"mounts": []any{map[string]any{"percent": float64(91)}, map[string]any{"percent": float64(20)}}},
		"services": {"services": []any{
			map[string]any{"exists": true, "active": true, "state": "running"},
			map[string]any{"exists": true, "active": false, "state": "failed"},
		}},
		"network": {"interfaces": []any{
			map[string]any{"state": "up"}, map[string]any{"state": "down"},
		}},
	}}
	snapshot, err := New(backend).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Hostname != "srv1" || snapshot.System != "Debian 13" || snapshot.Uptime != "1 j 1 h 1 min" || len(snapshot.Information) != 7 || len(snapshot.Cards) != 7 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if snapshot.Cards[2].Value != "52,4 °C" || snapshot.Cards[3].Value != "1 234" || snapshot.Cards[4].Status != "danger" || snapshot.Cards[5].Value != "1 / 2 actifs" || snapshot.Cards[6].Status != "warning" {
		t.Fatalf("unexpected supervision cards: %#v", snapshot.Cards)
	}
}

func TestSnapshotKeepsPartialFailuresVisible(t *testing.T) {
	backend := fakeBackend{data: map[string]map[string]any{
		"system": {"host": map[string]any{"hostname": "srv1"}, "cpu": map[string]any{}, "memory": map[string]any{}},
	}, failures: map[string]bool{"storage": true, "services": true, "network": true}}
	snapshot, err := New(backend).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Hostname != "srv1" || snapshot.Cards[4].Value != "Indisponible" || snapshot.Cards[6].Status != "neutral" {
		t.Fatalf("partial failure was not represented: %#v", snapshot)
	}
}

func TestSupervisionRefreshesOnlySupervisionDomains(t *testing.T) {
	backend := fakeBackend{data: map[string]map[string]any{
		"storage":  {"mounts": []any{map[string]any{"percent": float64(40)}}},
		"services": {"services": []any{map[string]any{"exists": true, "active": true, "state": "running"}}},
		"network":  {"interfaces": []any{map[string]any{"state": "up"}}},
	}}
	cards, err := New(backend).Supervision(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 3 || cards[0].ID != "storage" || cards[1].ID != "services" || cards[2].ID != "network" {
		t.Fatalf("unexpected supervision cards: %#v", cards)
	}
}

type fakeBackend struct {
	data     map[string]map[string]any
	failures map[string]bool
}

func (f fakeBackend) Execute(request protocol.Request) (protocol.Reply, error) {
	if f.failures[request.Domain] {
		return protocol.Reply{}, errors.New("unavailable")
	}
	return protocol.Reply{Response: api.Success(f.data[request.Domain])}, nil
}
