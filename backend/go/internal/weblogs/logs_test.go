package weblogs

import (
	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
	"context"
	"testing"
)

func TestLogsSnapshotOnlyRequestsListedSource(t *testing.T) {
	backend := &fakeBackend{}
	snapshot, err := New(backend).LogsSnapshot(context.Background(), "apache2/error.log", 750)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Selected != "apache2/error.log" || len(snapshot.Lines) != 2 || snapshot.Lines[0] != "new" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if got := backend.requests[1].Arguments; len(got) != 2 || got[0] != "apache2/error.log" || got[1] != "750" {
		t.Fatalf("unexpected tail arguments: %#v", got)
	}
}

func TestLogsSnapshotRejectsUnsafeLineCount(t *testing.T) {
	backend := &fakeBackend{}
	if _, err := New(backend).LogsSnapshot(context.Background(), "apache2/error.log", 5001); err == nil {
		t.Fatal("line count above backend limit must be rejected")
	}
	if len(backend.requests) != 0 {
		t.Fatalf("backend must not be called: %#v", backend.requests)
	}
}

type fakeBackend struct{ requests []protocol.Request }

func (f *fakeBackend) Execute(request protocol.Request) (protocol.Reply, error) {
	f.requests = append(f.requests, request)
	if request.Command == "list" {
		return protocol.Reply{Response: api.Success(map[string]any{"logs": []any{map[string]any{"id": "apache2/access.log"}, map[string]any{"id": "apache2/error.log"}}})}, nil
	}
	return protocol.Reply{Response: api.Success(map[string]any{"lines": []any{"old", "new"}})}, nil
}
