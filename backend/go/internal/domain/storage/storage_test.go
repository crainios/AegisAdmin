package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeCollector struct {
	mounts []Mount
	err    error
}

func (f fakeCollector) Mounts(context.Context) ([]Mount, error) { return f.mounts, f.err }

func TestNormalizeFindmntResponse(t *testing.T) {
	raw := []byte(`{"filesystems":[{"source":"/dev/sda1","target":"/","fstype":"ext4","size":1000,"used":400,"avail":600,"use%":"40%"},{"source":"tmpfs","target":"/run","fstype":"tmpfs","size":10,"used":1,"avail":9,"use%":"10%"},{"source":"/dev/sdb1","target":"/var/www/data","fstype":"xfs","size":2000,"used":500,"avail":1500,"use%":"25%"}]}`)
	mounts, err := normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 2 {
		t.Fatalf("unexpected mounts: %#v", mounts)
	}
	if mounts[0].ID != "data" || mounts[1].ID != "root" {
		t.Fatalf("unexpected identifiers: %#v", mounts)
	}
	if mounts[1].Percent != 40 || mounts[1].Available != 600 {
		t.Fatalf("unexpected root mount: %#v", mounts[1])
	}
}

func TestNormalizeAddsStableCollisionSuffix(t *testing.T) {
	raw := []byte(`{"filesystems":[{"source":"a","target":"/a b","fstype":"ext4","size":1,"used":0,"avail":1,"use%":"0%"},{"source":"b","target":"/a-b","fstype":"ext4","size":1,"used":0,"avail":1,"use%":"0%"}]}`)
	mounts, err := normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if mounts[0].ID == mounts[1].ID || len(mounts[0].ID) != len("a-b-")+12 {
		t.Fatalf("invalid collision identifiers: %#v", mounts)
	}
}

func TestNormalizeDeduplicatesSameMountTarget(t *testing.T) {
	raw := []byte(`{"filesystems":[{"source":"/dev/root","target":"/var","fstype":"ext4","size":100,"used":40,"avail":60,"use%":"40%"},{"source":"/dev/root[/var]","target":"/var","fstype":"ext4","size":100,"used":41,"avail":59,"use%":"41%"}]}`)
	mounts, err := normalize(raw)
	if err != nil || len(mounts) != 1 {
		t.Fatalf("mounts=%#v err=%v", mounts, err)
	}
	if mounts[0].ID != "var" || mounts[0].Source != "/dev/root[/var]" || mounts[0].Percent != 41 {
		t.Fatalf("unexpected deduplicated mount: %#v", mounts[0])
	}
}

func TestNormalizeUsesConfiguredDataMount(t *testing.T) {
	raw := []byte(`{"filesystems":[{"source":"/dev/sdb1","target":"/srv/aegis-data","fstype":"xfs","size":2000,"used":500,"avail":1500,"use%":"25%"}]}`)
	mounts, err := normalizeWithDataMount(raw, "/srv/aegis-data")
	if err != nil || len(mounts) != 1 || mounts[0].ID != "data" {
		t.Fatalf("mounts=%#v err=%v", mounts, err)
	}
}

func TestNormalizeAcceptsValidPartialOutputFromFindmnt(t *testing.T) {
	raw := []byte(`{"filesystems":[{"source":"/dev/sda1","target":"/","fstype":"ext4","size":1000,"used":400,"avail":600,"use%":"40%"}]}`)
	mounts, err := normalizeFindmntResult(raw, errors.New("findmnt partial failure"), defaultDataMount)
	if err != nil || len(mounts) != 1 || mounts[0].ID != "root" {
		t.Fatalf("mounts=%#v err=%v", mounts, err)
	}
}

func TestResolvePhysicalDevicesFollowsPartitionAndLogicalVolume(t *testing.T) {
	sysRoot := t.TempDir()
	disk := filepath.Join(sysRoot, "devices", "pci", "block", "sda")
	partition := filepath.Join(disk, "sda2")
	logical := filepath.Join(sysRoot, "devices", "virtual", "block", "dm-0")
	for _, directory := range []string{partition, filepath.Join(logical, "slaves"), filepath.Join(sysRoot, "dev", "block")} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(partition, "partition"), []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(partition, filepath.Join(logical, "slaves", "sda2")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(logical, filepath.Join(sysRoot, "dev", "block", "253:0")); err != nil {
		t.Fatal(err)
	}
	if got := resolvePhysicalDevices("253:0", sysRoot); got != "/dev/sda" {
		t.Fatalf("physical device = %q", got)
	}
}

func TestStorageProfile(t *testing.T) {
	directory := t.TempDir()
	profile := filepath.Join(directory, "storage")
	if err := os.WriteFile(profile, []byte("findmnt=/bin/findmnt\ndata_mount=/srv/aegis-data\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	collector := &LinuxCollector{findmnt: "/usr/bin/findmnt", dataMount: defaultDataMount}
	collector.loadProfile(profile)
	if collector.findmnt != "/bin/findmnt" || collector.dataMount != "/srv/aegis-data" {
		t.Fatalf("collector = %#v", collector)
	}
}

func TestHandlerStatus(t *testing.T) {
	handler := New(fakeCollector{mounts: []Mount{{ID: "root", Source: "/dev/sda1", Mount: "/", Filesystem: "ext4", Size: 100, Available: 100}}})
	reply := handler.Handle(context.Background(), "status", []string{"root"})
	if reply.ExitCode != 0 || !reply.Response.Success || reply.Response.Data == nil {
		t.Fatalf("unexpected reply: %#v", reply)
	}
	if (*reply.Response.Data)["id"] != "root" {
		t.Fatalf("unexpected data: %#v", *reply.Response.Data)
	}
}

func TestHandlerErrors(t *testing.T) {
	handler := New(fakeCollector{})
	tests := []struct {
		command   string
		arguments []string
		exit      int
		code      string
	}{
		{"", nil, 2, "MISSING_COMMAND"},
		{"unknown", nil, 4, "COMMAND_NOT_FOUND"},
		{"status", []string{"../root"}, 2, "INVALID_STORAGE_IDENTIFIER"},
		{"status", []string{"root"}, 5, "STORAGE_NOT_FOUND"},
	}
	for _, test := range tests {
		reply := handler.Handle(context.Background(), test.command, test.arguments)
		if reply.ExitCode != test.exit || reply.Response.Error == nil || reply.Response.Error.Code != test.code {
			t.Fatalf("unexpected reply: %#v", reply)
		}
	}
}

func TestStorageReadFailureIncludesDiagnostic(t *testing.T) {
	handler := New(fakeCollector{err: errors.New("findmnt command failed: operation not permitted")})
	reply := handler.Handle(context.Background(), "list", nil)
	if reply.Response.Error == nil || reply.Response.Error.Details["reason"] != "findmnt command failed: operation not permitted" {
		t.Fatalf("unexpected reply: %#v", reply)
	}
}

func TestLinuxCollectorOnCurrentHost(t *testing.T) {
	mounts, err := NewLinuxCollector().Mounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) == 0 {
		t.Fatal("at least one real mount is expected")
	}
}
