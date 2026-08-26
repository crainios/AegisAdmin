package configuration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

type fakeProvider struct{}

func (fakeProvider) Handle(_ context.Context, command string, _ []string) protocol.Reply {
	return protocol.Reply{Response: api.Success(map[string]any{"command": command})}
}

type fakePlatform struct{}

func (fakePlatform) Inventory(context.Context) map[string]any {
	return map[string]any{"packages": map[string]any{"count": 1}, "accounts": map[string]any{"user_count": 1}, "ports": map[string]any{"listeners": []string{}}}
}

func TestCapabilitiesAreReadOnly(t *testing.T) {
	reply := New(map[string]Provider{"system": fakeProvider{}}, fakePlatform{}).Handle(context.Background(), "capabilities", nil)
	if !reply.Response.Success || reply.Response.Data == nil || (*reply.Response.Data)["inventory_read_only"] != true || (*reply.Response.Data)["snapshot_storage"] != true {
		t.Fatalf("unexpected response: %#v", reply)
	}
}

func TestSnapshotCreateListAndRead(t *testing.T) {
	providers := map[string]Provider{}
	for _, section := range sections {
		if section.provider != "" {
			providers[section.provider] = fakeProvider{}
		}
	}
	store := newSnapshotStore(t.TempDir())
	store.now = func() time.Time { return time.Date(2026, 8, 16, 18, 30, 0, 0, time.UTC) }
	handler := newHandler(providers, fakePlatform{}, store)
	created := handler.Handle(context.Background(), "snapshot-create", nil)
	if !created.Response.Success || created.Response.Data == nil {
		t.Fatalf("create failed: %#v", created)
	}
	snapshot, ok := (*created.Response.Data)["snapshot"].(Snapshot)
	if !ok || snapshot.ID == "" {
		t.Fatalf("unexpected snapshot: %#v", (*created.Response.Data)["snapshot"])
	}
	listed := handler.Handle(context.Background(), "snapshots", nil)
	if !listed.Response.Success || listed.Response.Data == nil || (*listed.Response.Data)["count"] != 1 {
		t.Fatalf("list failed: %#v", listed)
	}
	read := handler.Handle(context.Background(), "snapshot", []string{snapshot.ID})
	if !read.Response.Success {
		t.Fatalf("read failed: %#v", read)
	}
	info, err := os.Stat(filepath.Join(store.directory, snapshot.ID+".json"))
	if err != nil || info.Mode().Perm() != 0440 {
		t.Fatalf("unexpected snapshot permissions: info=%#v err=%v", info, err)
	}
}

func TestAlteredSnapshotIsRejected(t *testing.T) {
	store := newSnapshotStore(t.TempDir())
	store.now = func() time.Time { return time.Date(2026, 8, 16, 18, 30, 0, 0, time.UTC) }
	snapshot, err := store.create(map[string]any{
		"schema":   "aegisadmin.configuration.inventory.v1",
		"sections": []map[string]any{{"available": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.directory, snapshot.ID+".json")
	if err = os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content[len(content)-2] = ' '
	if err = os.WriteFile(path, content, 0440); err != nil {
		t.Fatal(err)
	}
	if _, err = store.get(snapshot.ID); err == nil {
		t.Fatal("altered snapshot was accepted")
	}
}

func TestSnapshotPreservesLargeIntegerFingerprint(t *testing.T) {
	store := newSnapshotStore(t.TempDir())
	store.now = func() time.Time { return time.Date(2026, 8, 16, 18, 30, 0, 0, time.UTC) }
	snapshot, err := store.create(map[string]any{
		"schema": "aegisadmin.configuration.inventory.v1",
		"sections": []map[string]any{{
			"available": true,
			"data":      map[string]any{"counter": uint64(9007199254740993)},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.get(snapshot.ID); err != nil {
		t.Fatalf("intact snapshot with a large integer was rejected: %v", err)
	}
}

func TestSnapshotRenameImportCompareAndDelete(t *testing.T) {
	firstStore := newSnapshotStore(t.TempDir())
	firstStore.now = func() time.Time { return time.Date(2026, 8, 16, 18, 30, 0, 0, time.UTC) }
	first, err := firstStore.create(map[string]any{"schema": "aegisadmin.configuration.inventory.v1", "sections": []map[string]any{{"id": "php", "label": "PHP", "data": map[string]any{"version": "8.4"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = firstStore.rename(first.ID, "Serveur A"); err != nil {
		t.Fatal(err)
	}
	if metadata := firstStore.metadata(first.ID); metadata.Name != "Serveur A" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	content, err := os.ReadFile(filepath.Join(firstStore.directory, first.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	secondStore := newSnapshotStore(t.TempDir())
	imported, err := secondStore.importSnapshot(content, "Serveur importé")
	if err != nil {
		t.Fatal(err)
	}
	if !secondStore.metadata(imported.ID).Imported {
		t.Fatal("import flag missing")
	}
	secondStore.now = func() time.Time { return time.Date(2026, 8, 16, 18, 31, 0, 0, time.UTC) }
	second, err := secondStore.create(map[string]any{"schema": "aegisadmin.configuration.inventory.v1", "sections": []map[string]any{{"id": "php", "label": "PHP", "data": map[string]any{"version": "8.5"}}}})
	if err != nil {
		t.Fatal(err)
	}
	comparison := compareSnapshots(imported, second)
	counts := comparison["counts"].(map[string]int)
	if counts["different"] != 1 {
		t.Fatalf("unexpected comparison: %#v", comparison)
	}
	if err = secondStore.delete(imported.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = secondStore.get(imported.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot still exists: %v", err)
	}
}

func TestInventoryHasVersionedSchemaAndAllSections(t *testing.T) {
	providers := map[string]Provider{}
	for _, section := range sections {
		if section.provider != "" {
			providers[section.provider] = fakeProvider{}
		}
	}
	reply := New(providers, fakePlatform{}).Handle(context.Background(), "inventory", nil)
	if !reply.Response.Success || reply.Response.Data == nil {
		t.Fatalf("unexpected response: %#v", reply)
	}
	if (*reply.Response.Data)["schema"] != "aegisadmin.configuration.inventory.v1" {
		t.Fatalf("unexpected schema: %#v", *reply.Response.Data)
	}
	items, ok := (*reply.Response.Data)["sections"].([]map[string]any)
	if !ok || len(items) != len(sections) {
		t.Fatalf("unexpected sections: %#v", (*reply.Response.Data)["sections"])
	}
}

func TestMutationCommandsAreRejected(t *testing.T) {
	reply := New(nil, fakePlatform{}).Handle(context.Background(), "apply", nil)
	if reply.ExitCode != 4 || reply.Response.Error == nil || reply.Response.Error.Code != "COMMAND_NOT_FOUND" {
		t.Fatalf("unexpected response: %#v", reply)
	}
}

func TestParseListenerStructuresIPv4AndIPv6(t *testing.T) {
	ipv4, ok := parseListener("tcp LISTEN 0 4096 127.0.0.1:9080 0.0.0.0:*")
	if !ok || ipv4["protocol"] != "tcp" || ipv4["address"] != "127.0.0.1" || ipv4["port"] != "9080" {
		t.Fatalf("IPv4 listener = %#v, %v", ipv4, ok)
	}
	ipv6, ok := parseListener("tcp LISTEN 0 511 [::]:8443 [::]:*")
	if !ok || ipv6["address"] != "::" || ipv6["port"] != "8443" {
		t.Fatalf("IPv6 listener = %#v, %v", ipv6, ok)
	}
}
