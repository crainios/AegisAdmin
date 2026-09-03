package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseProcessesSortsAndNormalizes(t *testing.T) {
	output := "200 redis Ssl 0.6 0.1 14524 213578 redis-server\n" +
		"50 root S 0.0 0.0 4096 300000 cron\n" +
		"100 mysql Ssl 4.3 11.9 893704 184275 mysqld\n"

	processes, err := parseProcesses(output, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(processes) != 3 {
		t.Fatalf("unexpected process count: %d", len(processes))
	}
	if processes[0].PID != 100 || processes[1].PID != 200 || processes[2].PID != 50 {
		t.Fatalf("unexpected process order: %#v", processes)
	}
	if processes[0].MemoryBytes != 915152896 {
		t.Fatalf("unexpected memory conversion: %d", processes[0].MemoryBytes)
	}
}

func TestParseProcessesRejectsInvalidMetrics(t *testing.T) {
	_, err := parseProcesses("100 root S -1.0 0.0 1 1 process\n", -1)
	if err == nil {
		t.Fatal("negative CPU usage must be rejected")
	}
}

func TestProcessDescriptionSourcesAreParsedSafely(t *testing.T) {
	root := t.TempDir()
	cgroup := filepath.Join(root, "cgroup")
	if err := os.WriteFile(cgroup, []byte("0::/system.slice/ssh.service\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if unit := readProcessUnit(cgroup); unit != "ssh.service" {
		t.Fatalf("unit=%q", unit)
	}
	descriptions := parseSystemdDescriptions("Description=OpenSSH server daemon\nId=ssh.service\n\nId=cron.service\nDescription=Regular background program processing daemon\n")
	if descriptions["ssh.service"] != "OpenSSH server daemon" || descriptions["cron.service"] == "" {
		t.Fatalf("descriptions=%#v", descriptions)
	}
	if knownProcessDescription("aegisadmin-web") == "" || knownProcessDescription("private-worker") != "" {
		t.Fatal("internal process fallback is not selective")
	}
}

func TestReadMemory(t *testing.T) {
	path := writeFixture(t, "meminfo", "MemTotal: 1000 kB\nMemAvailable: 400 kB\nBuffers: 50 kB\nCached: 200 kB\nSReclaimable: 25 kB\nShmem: 10 kB\n")

	memory, err := readMemory(path)
	if err != nil {
		t.Fatal(err)
	}
	if memory["total_bytes"] != int64(1024000) || memory["used_bytes"] != int64(614400) {
		t.Fatalf("unexpected memory values: %#v", memory)
	}
	if memory["cached_bytes"] != int64(271360) || memory["percent"] != 60.0 {
		t.Fatalf("unexpected memory normalization: %#v", memory)
	}
}

func TestReadOSRelease(t *testing.T) {
	path := writeFixture(t, "os-release", "NAME=Ubuntu\nPRETTY_NAME=\"Ubuntu 26.04 LTS\"\nID=ubuntu\n")

	values, err := readOSRelease(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["NAME"] != "Ubuntu" || values["PRETTY_NAME"] != "Ubuntu 26.04 LTS" {
		t.Fatalf("unexpected OS release: %#v", values)
	}
}

func TestCPUUsageBetweenSamples(t *testing.T) {
	usage := cpuUsageBetween(
		cpuTimes{total: 100, idle: 60},
		cpuTimes{total: 200, idle: 100},
	)
	if usage != 60.0 {
		t.Fatalf("unexpected CPU usage: %v", usage)
	}
	if value := cpuUsageBetween(cpuTimes{total: 100, idle: 50}, cpuTimes{total: 100, idle: 50}); value != 0 {
		t.Fatalf("usage without elapsed CPU time: %v", value)
	}
}

func TestCPUSamplerUpdatesWithoutWaitingInReader(t *testing.T) {
	path := writeFixture(t, "stat", "cpu 10 0 10 80 0 0 0 0\n")
	first, err := readCPUTimes(path)
	if err != nil {
		t.Fatal(err)
	}
	sampler := &cpuSampler{path: path, previous: first, hasValue: true}
	if err := os.WriteFile(path, []byte("cpu 30 0 30 140 0 0 0 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sampler.sample()
	usage, err := sampler.value()
	if err != nil || usage != 40.0 {
		t.Fatalf("usage=%v err=%v", usage, err)
	}
}

func TestLinuxCollectorOnCurrentHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collector := NewLinuxCollector(ctx)
	info, err := collector.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info["backend"] != backendVersion {
		t.Fatalf("unexpected backend version: %#v", info["backend"])
	}
	if _, ok := info["host"].(map[string]any); !ok {
		t.Fatalf("invalid host data: %#v", info["host"])
	}

	listing, err := collector.Processes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if listing["limit"] != processLimit {
		t.Fatalf("unexpected process limit: %#v", listing["limit"])
	}
}

func writeFixture(t *testing.T, name string, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
