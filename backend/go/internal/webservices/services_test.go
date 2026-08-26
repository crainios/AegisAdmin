package webservices

import (
	"context"
	"testing"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

func TestServicesSnapshotClassifiesServices(t *testing.T) {
	backend := &fakeBackend{reply: protocol.Reply{Response: api.Success(map[string]any{"services": []any{
		map[string]any{"id": "apache2", "exists": true, "active": true, "enabled": true, "state": "running"},
		map[string]any{"id": "mariadb", "exists": true, "active": false, "enabled": true, "state": "failed"},
		map[string]any{"id": "tor", "exists": false, "active": false, "enabled": false, "state": "not-found"},
	}})}}
	snapshot, err := New(backend).ServicesSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Summary.Total != 3 || snapshot.Summary.Installed != 2 || snapshot.Summary.Active != 1 || snapshot.Summary.Inactive != 1 || snapshot.Summary.Missing != 1 {
		t.Fatalf("unexpected summary: %#v", snapshot.Summary)
	}
	if snapshot.Services[1].Status != "danger" || snapshot.Services[2].StatusLabel != "Non installé" {
		t.Fatalf("unexpected services: %#v", snapshot.Services)
	}
}

func TestRestartServiceUsesBoundedRequest(t *testing.T) {
	backend := &fakeBackend{reply: protocol.Reply{Response: api.Success(map[string]any{})}}
	if err := New(backend).RestartService(context.Background(), "php8.5-fpm"); err != nil {
		t.Fatal(err)
	}
	if backend.request.Domain != "services" || backend.request.Command != "restart" || len(backend.request.Arguments) != 1 || backend.request.Arguments[0] != "php8.5-fpm" {
		t.Fatalf("unexpected request: %#v", backend.request)
	}
	if err := New(backend).RestartService(context.Background(), "../../etc/passwd"); err == nil {
		t.Fatal("invalid identifier was accepted")
	}
}

type fakeBackend struct {
	reply   protocol.Reply
	request protocol.Request
}

func (f *fakeBackend) Execute(request protocol.Request) (protocol.Reply, error) {
	f.request = request
	return f.reply, nil
}
