package webphp

import (
	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
	"context"
	"testing"
)

func TestPHPSnapshotAndRestart(t *testing.T) {
	backend := &fakeBackend{}
	snapshot, err := New(backend).PHPSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CLI.Version != "8.5.9" || len(snapshot.Instances) != 1 || snapshot.Instances[0].Status != "success" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if err = New(backend).RestartPHP(context.Background(), "fpm-8.5"); err != nil {
		t.Fatal(err)
	}
	if request := backend.requests[len(backend.requests)-1]; request.Command != "restart" || request.Arguments[0] != "fpm-8.5" {
		t.Fatalf("unexpected restart: %#v", request)
	}
}

type fakeBackend struct{ requests []protocol.Request }

func (f *fakeBackend) Execute(request protocol.Request) (protocol.Reply, error) {
	f.requests = append(f.requests, request)
	data := map[string]any{}
	switch request.Command {
	case "info":
		data = map[string]any{"version": "8.5.9", "sapi": "cli", "ini_file": "/etc/php.ini", "scan_dir": "/etc/php.d"}
	case "fpm":
		data = map[string]any{"instances": []any{map[string]any{"id": "fpm-8.5", "version": "8.5", "service": "php8.5-fpm", "exists": true, "active": true, "enabled": true, "state": "running"}}}
	}
	return protocol.Reply{Response: api.Success(data)}, nil
}
