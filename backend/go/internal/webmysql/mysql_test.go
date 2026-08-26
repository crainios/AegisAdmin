package webmysql

import (
	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
	"context"
	"testing"
)

func TestSnapshotAndRestart(t *testing.T) {
	b := &fakeBackend{}
	s, e := New(b).MySQLSnapshot(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if s.Server.Product != "MariaDB" || s.Metrics["threads_connected"] != 3 || len(s.Databases) != 1 {
		t.Fatalf("unexpected: %#v", s)
	}
	if e = New(b).RestartMySQL(context.Background()); e != nil {
		t.Fatal(e)
	}
	if b.requests[3].Command != "restart" {
		t.Fatalf("requests: %#v", b.requests)
	}
}

type fakeBackend struct{ requests []protocol.Request }

func (f *fakeBackend) Execute(r protocol.Request) (protocol.Reply, error) {
	f.requests = append(f.requests, r)
	d := map[string]any{}
	switch r.Command {
	case "info":
		d = map[string]any{"product": "MariaDB", "version": "11.8", "hostname": "db", "port": 3306, "socket": "/run/mysql.sock", "data_directory": "/var/lib/mysql", "default_storage_engine": "InnoDB", "service": map[string]any{"unit": "mariadb.service", "service": "mariadb", "exists": true, "active": true, "enabled": true, "state": "running"}}
	case "status":
		d = map[string]any{"metrics": map[string]any{"threads_connected": 3, "threads_running": 1, "queries": 42, "slow_queries": 0, "uptime": 3600}}
	case "databases":
		d = map[string]any{"databases": []any{map[string]any{"name": "app", "size_bytes": 1024, "table_count": 5, "kind": "user"}}}
	}
	return protocol.Reply{Response: api.Success(d)}, nil
}
