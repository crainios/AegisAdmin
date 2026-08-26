package webtor

import (
	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
	"context"
	"testing"
)

func TestSnapshotAndActions(t *testing.T) {
	b := &fakeBackend{}
	s, e := New(b).TorSnapshot(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if s.Info.Version != "0.4.8" || !s.Status.Active || !s.ConfigurationValid || len(s.Services) != 1 {
		t.Fatalf("snapshot=%#v", s)
	}
	if e = New(b).TorAction(context.Background(), "reload"); e != nil {
		t.Fatal(e)
	}
	if b.requests[4].Command != "reload" {
		t.Fatalf("requests=%#v", b.requests)
	}
}

type fakeBackend struct{ requests []protocol.Request }

func (f *fakeBackend) Execute(r protocol.Request) (protocol.Reply, error) {
	f.requests = append(f.requests, r)
	d := map[string]any{}
	switch r.Command {
	case "info":
		d = map[string]any{"product": "Tor", "version": "0.4.8", "config_file": "/etc/tor/torrc", "service": "tor@default", "unit": "tor@default.service"}
	case "status":
		d = map[string]any{"exists": true, "active": true, "enabled": true, "load_state": "loaded", "active_state": "active", "state": "running", "main_pid": 12, "memory_bytes": 1024, "tasks": 4, "bootstrap_percent": 100}
	case "configtest":
		d = map[string]any{"valid": true, "message": "Configuration valide"}
	case "hidden-services":
		d = map[string]any{"services": []any{map[string]any{"id": "site", "hostname": "example.onion", "ports": []any{}}}}
	}
	return protocol.Reply{Response: api.Success(d)}, nil
}
