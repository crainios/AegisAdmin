package webstorage

import (
	"context"
	"errors"
	"testing"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

func TestSnapshotFormatsAndClassifiesMounts(t *testing.T) {
	backend := fakeBackend{reply: protocol.Reply{Response: api.Success(map[string]any{
		"mounts": []any{
			map[string]any{"id": "root", "source": "/dev/root", "mount": "/", "filesystem": "ext4", "size": int64(100 << 30), "used": int64(81 << 30), "available": int64(19 << 30), "percent": 81},
			map[string]any{"id": "data", "source": "/dev/sda1", "mount": "/var/www/data", "filesystem": "ext4", "size": int64(2 << 40), "used": int64(1 << 40), "available": int64(1 << 40), "percent": 50},
		},
	})}}
	snapshot, err := New(backend).StorageSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Summary.Total != 2 || snapshot.Summary.Warning != 1 || snapshot.Summary.Normal != 1 ||
		snapshot.Mounts[0].StatusLabel != "À surveiller" || snapshot.Mounts[1].SizeLabel != "2,0 Tio" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestSnapshotRejectsBackendFailure(t *testing.T) {
	if _, err := New(fakeBackend{err: errors.New("offline")}).StorageSnapshot(context.Background()); err == nil {
		t.Fatal("backend failure was accepted")
	}
}

type fakeBackend struct {
	reply protocol.Reply
	err   error
}

func (f fakeBackend) Execute(protocol.Request) (protocol.Reply, error) {
	return f.reply, f.err
}
