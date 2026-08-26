package webcron

import (
	"context"
	"testing"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

type fakeBackend struct{ requests []protocol.Request }

func (f *fakeBackend) Execute(request protocol.Request) (protocol.Reply, error) {
	f.requests = append(f.requests, request)
	data := map[string]any{}
	switch request.Command {
	case "info":
		data = map[string]any{"product": "Cron", "version": "3.0", "service": "cron", "unit": "cron.service", "anacron_available": true}
	case "status":
		data = map[string]any{"exists": true, "active": true, "enabled": true, "main_pid": 12, "memory_bytes": 1024, "tasks": 2}
	case "users":
		data = map[string]any{"users": []any{map[string]any{"name": "root", "system": false}}}
	case "jobs":
		data = map[string]any{"jobs": []any{map[string]any{"id": "legacy-1234567890abcdef1234567890abcdef", "user": "root", "schedule": "@daily", "command": "/bin/true", "enabled": true, "editable": true}}}
	case "run":
		data = map[string]any{"execution_id": "1234567890abcdef1234567890abcdef"}
	case "run-result":
		exit := 0
		data = map[string]any{"execution_id": request.Arguments[0], "status": "finished", "exit_code": exit, "timed_out": false, "truncated": false, "duration_ms": 12, "stdout": "ok", "stderr": ""}
	}
	return protocol.Reply{Response: api.Success(data)}, nil
}
func TestSnapshotAndRun(t *testing.T) {
	backend := &fakeBackend{}
	client := New(backend)
	snapshot, err := client.CronSnapshot(context.Background())
	if err != nil || len(snapshot.Jobs) != 1 || snapshot.Jobs[0].Command != "/bin/true" {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	id, err := client.CronAction(context.Background(), "run", "root", snapshot.Jobs[0].ID, "", "")
	if err != nil || id != "1234567890abcdef1234567890abcdef" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if got := backend.requests[len(backend.requests)-1]; got.Command != "run" || len(got.Arguments) != 2 {
		t.Fatalf("request=%#v", got)
	}
	result, err := client.CronResult(context.Background(), id)
	if err != nil || result.Status != "finished" || result.ExitCode == nil || *result.ExitCode != 0 || result.Stdout != "ok" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
