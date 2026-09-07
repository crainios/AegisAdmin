package downloadstats

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCollectCountsOnlySuccessfulDebDownloads(t *testing.T) {
	directory := t.TempDir()
	log := filepath.Join(directory, "access.log")
	content := "192.0.2.1 - - [06/Sep/2026:10:00:00 +0200] \"GET /pool/main/a/aegisadmin/aegisadmin_0.2.84_amd64.deb HTTP/1.1\" 200 10 \"-\" \"Debian APT-HTTP/1.3\"\n" +
		"192.0.2.2 - - [06/Sep/2026:11:00:00 +0200] \"GET /pool/main/a/aegisadmin/aegisadmin_0.2.84_amd64.deb HTTP/1.1\" 206 5 \"-\" \"Debian APT-HTTP/1.3\"\n" +
		"192.0.2.3 - - [07/Sep/2026:12:00:00 +0200] \"GET /pool/main/a/aegisadmin/aegisadmin_0.2.85_arm64.deb HTTP/1.1\" 404 0 \"-\" \"curl\"\n" +
		"192.0.2.3 - - [07/Sep/2026:12:00:00 +0200] \"HEAD /pool/main/a/aegisadmin/aegisadmin_0.2.84_amd64.deb HTTP/1.1\" 200 0 \"-\" \"curl\"\n" +
		"192.0.2.4 - - [07/Sep/2026:12:00:00 +0200] \"GET /dists/stable/InRelease HTTP/1.1\" 200 10 \"-\" \"APT\"\n"
	if err := os.WriteFile(log, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	stats, err := Collect(filepath.Join(directory, "access.log*"), time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalAPTDownloads != 1 || len(stats.Days) != 1 || stats.Days[0].Downloads != 1 {
		t.Fatalf("unexpected totals: %+v", stats)
	}
	if len(stats.Releases) != 1 || stats.Releases[0].Version != "0.2.84" || stats.Releases[0].Architecture != "amd64" || stats.Releases[0].APTDownloads != 1 {
		t.Fatalf("unexpected releases: %+v", stats.Releases)
	}
}

func TestCollectReadsCompressedRotatedLogs(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "access.log.2.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(file)
	line := "192.0.2.1 - - [05/Sep/2026:10:00:00 +0200] \"GET /pool/aegisadmin_0.2.83_amd64.deb HTTP/1.1\" 200 10 \"-\" \"APT\"\n"
	if _, err = compressed.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
	if err = compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	stats, err := Collect(filepath.Join(directory, "access.log*"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalAPTDownloads != 1 || stats.FilesRead != 1 {
		t.Fatalf("unexpected compressed log totals: %+v", stats)
	}
}

func TestCollectRejectsMissingLogs(t *testing.T) {
	if _, err := Collect(filepath.Join(t.TempDir(), "missing*"), time.Now()); err == nil {
		t.Fatal("missing logs must fail")
	}
}

func TestCollectSerializesEmptyCollectionsAsArrays(t *testing.T) {
	directory := t.TempDir()
	log := filepath.Join(directory, "access.log")
	if err := os.WriteFile(log, nil, 0600); err != nil {
		t.Fatal(err)
	}
	stats, err := Collect(log, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(content)
	if !strings.Contains(serialized, `"days":[]`) || !strings.Contains(serialized, `"releases":[]`) {
		t.Fatalf("empty collections must be JSON arrays: %s", serialized)
	}
}
