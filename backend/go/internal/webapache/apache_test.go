package webapache

import (
	"context"
	"testing"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

func TestOverviewAndAction(t *testing.T) {
	backend := &fakeBackend{}
	snapshot, err := New(backend).ApacheSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != "2.4.65" || !snapshot.ConfigValid || len(snapshot.VHosts) != 1 || len(snapshot.Sites) != 1 || len(snapshot.Modules) != 1 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if err = New(backend).ApacheAction(context.Background(), "reload"); err != nil {
		t.Fatal(err)
	}
	if backend.requests[1].Command != "reload" {
		t.Fatalf("requests=%#v", backend.requests)
	}
	config, err := New(backend).ApacheSite(context.Background(), "abc")
	if err != nil || config.Filename != "site.conf" {
		t.Fatalf("config=%#v err=%v", config, err)
	}
	if err = New(backend).ApacheSiteAction(context.Background(), "update", "abc", "", "Syntax OK\n"); err != nil {
		t.Fatal(err)
	}
	if backend.requests[3].Command != "update" || len(backend.requests[3].Arguments) != 1 {
		t.Fatalf("requests=%#v", backend.requests)
	}
}

type fakeBackend struct{ requests []protocol.Request }

func (f *fakeBackend) Execute(request protocol.Request) (protocol.Reply, error) {
	f.requests = append(f.requests, request)
	data := map[string]any{}
	if request.Command == "overview" {
		data = map[string]any{
			"info":       map[string]any{"version": "2.4.65", "built": "2026"},
			"configtest": map[string]any{"valid": true, "message": "Syntax OK"},
			"vhosts": map[string]any{"virtual_hosts": []any{
				map[string]any{"server_name": "example.fr", "port": 443, "config_file": "site.conf", "document_root": "/var/www/site"},
			}},
			"sites": map[string]any{"sites": []any{
				map[string]any{"filename": "site.conf", "enabled": true, "size_bytes": 123, "server_names": []any{"example.fr"}, "ports": []any{443}},
			}},
			"modules": map[string]any{"modules": []any{
				map[string]any{"name": "ssl", "type": "shared"},
			}},
		}
	} else if request.Command == "site" {
		data = map[string]any{"filename": "site.conf", "config_id": "abc", "enabled": true, "content": "Syntax OK\n"}
	}
	return protocol.Reply{Response: api.Success(data)}, nil
}
