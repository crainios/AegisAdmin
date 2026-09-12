package logs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeCollector struct {
	logs  []Log
	lines []string
	total int64
	err   error
}

func (f fakeCollector) Logs(context.Context) []Log { return f.logs }
func (f fakeCollector) Tail(context.Context, string, int) ([]string, int64, error) {
	return f.lines, f.total, f.err
}

type fixtureRunner struct {
	output []byte
	err    error
	calls  []string
}

func (f *fixtureRunner) Run(_ context.Context, arguments ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(arguments, " "))
	return f.output, f.err
}

func TestLinuxCollectorDiscoversAllowedLogs(t *testing.T) {
	root := t.TempDir()
	apache := filepath.Join(root, "apache2")
	mysql := filepath.Join(root, "mysql")
	if err := os.Mkdir(apache, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(mysql, 0750); err != nil {
		t.Fatal(err)
	}
	writeLog(t, filepath.Join(apache, "error.log"), "error\n")
	writeLog(t, filepath.Join(apache, "access.log.1"), "rotated\n")
	writeLog(t, filepath.Join(mysql, "mysql.log.gz"), "compressed\n")
	fail2ban := filepath.Join(root, "fail2ban.log")
	writeLog(t, fail2ban, "ban\n")
	if err := os.Symlink(filepath.Join(apache, "error.log"), filepath.Join(apache, "linked.log")); err != nil {
		t.Fatal(err)
	}

	collector := &LinuxCollector{
		root:        root,
		directories: []string{mysql, apache, filepath.Join(root, "absent")},
		files:       []string{fail2ban, filepath.Join(root, "missing.log")},
		runner:      &fixtureRunner{},
	}
	logs := collector.Logs(context.Background())
	if len(logs) != 2 || logs[0].ID != "apache2/error.log" || logs[1].ID != "fail2ban.log" {
		t.Fatalf("unexpected logs: %#v", logs)
	}
}

func TestLinuxCollectorTailsFixture(t *testing.T) {
	root := t.TempDir()
	apache := filepath.Join(root, "apache2")
	if err := os.Mkdir(apache, 0750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(apache, "error.log")
	writeLog(t, path, "readable")
	output, err := os.ReadFile("testdata/apache-error.log")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, output, 0640); err != nil {
		t.Fatal(err)
	}
	runner := &fixtureRunner{output: output}
	collector := &LinuxCollector{root: root, directories: []string{apache}, runner: runner}
	lines, total, err := collector.Tail(context.Background(), "apache2/error.log", 100)
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 || len(lines) != 3 || lines[1] != `quoted "message"` || lines[2] != "last line" {
		t.Fatalf("unexpected lines: %#v", lines)
	}
	expectedCall := "-n 100 -- " + path
	if len(runner.calls) != 1 || runner.calls[0] != expectedCall {
		t.Fatalf("unexpected tail calls: %#v", runner.calls)
	}
}

func TestLinuxCollectorTailErrors(t *testing.T) {
	root := t.TempDir()
	collector := &LinuxCollector{root: root, directories: []string{root}, runner: &fixtureRunner{}}
	if _, _, err := collector.Tail(context.Background(), "apache2/missing.log", 10); !errors.Is(err, errNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandlerListAndTail(t *testing.T) {
	handler := New(fakeCollector{
		logs:  []Log{{ID: "apache2/error.log", Path: "/var/log/apache2/error.log"}},
		lines: []string{"one", "two"},
		total: 42,
	})
	list := handler.Handle(context.Background(), "list", nil)
	if list.ExitCode != 0 || list.Response.Data == nil {
		t.Fatalf("unexpected list reply: %#v", list)
	}
	items := (*list.Response.Data)["logs"].([]map[string]any)
	if len(items) != 1 || len(items[0]) != 1 || items[0]["id"] != "apache2/error.log" {
		t.Fatalf("unexpected list contract: %#v", items)
	}

	tail := handler.Handle(context.Background(), "tail", []string{"apache2/error.log", "100"})
	if tail.ExitCode != 0 || tail.Response.Data == nil || (*tail.Response.Data)["requested_lines"] != 100 || (*tail.Response.Data)["total_lines"] != int64(42) {
		t.Fatalf("unexpected tail reply: %#v", tail)
	}
	lines := (*tail.Response.Data)["lines"].([]string)
	if len(lines) != 2 || lines[1] != "two" {
		t.Fatalf("unexpected tail lines: %#v", lines)
	}
}

func TestCountLinesIncludesFinalLineWithoutNewline(t *testing.T) {
	for input, expected := range map[string]int64{"": 0, "one": 1, "one\n": 1, "one\ntwo": 2, "one\ntwo\n": 2} {
		total, err := countLines(strings.NewReader(input))
		if err != nil || total != expected {
			t.Fatalf("countLines(%q) = %d, %v; want %d", input, total, err, expected)
		}
	}
}

func TestHandlerErrors(t *testing.T) {
	tests := []struct {
		collector fakeCollector
		command   string
		arguments []string
		exit      int
		code      string
	}{
		{fakeCollector{}, "", nil, 2, "MISSING_COMMAND"},
		{fakeCollector{}, "unknown", nil, 4, "COMMAND_NOT_FOUND"},
		{fakeCollector{}, "list", []string{"extra"}, 2, "INVALID_ARGUMENT_COUNT"},
		{fakeCollector{}, "tail", []string{"apache2/error.log"}, 2, "INVALID_ARGUMENT_COUNT"},
		{fakeCollector{}, "tail", []string{"../secret", "10"}, 2, "INVALID_LOG_IDENTIFIER"},
		{fakeCollector{}, "tail", []string{"apache2/error.log", "0"}, 2, "INVALID_LINE_COUNT"},
		{fakeCollector{}, "tail", []string{"apache2/error.log", "5001"}, 2, "INVALID_LINE_COUNT"},
		{fakeCollector{}, "tail", []string{"apache2/error.log", "ten"}, 2, "INVALID_LINE_COUNT"},
		{fakeCollector{err: errNotFound}, "tail", []string{"apache2/error.log", "10"}, 5, "LOG_NOT_FOUND"},
		{fakeCollector{err: errNotReadable}, "tail", []string{"apache2/error.log", "10"}, 7, "LOG_NOT_READABLE"},
		{fakeCollector{err: errors.New("tail failed")}, "tail", []string{"apache2/error.log", "10"}, 10, "LOG_READ_FAILED"},
	}
	for _, test := range tests {
		reply := New(test.collector).Handle(context.Background(), test.command, test.arguments)
		if reply.ExitCode != test.exit || reply.Response.Error == nil || reply.Response.Error.Code != test.code {
			t.Fatalf("unexpected reply for %q: %#v", test.command, reply)
		}
	}
}

func TestIdentifierAndRotationValidation(t *testing.T) {
	for _, identifier := range []string{"apache2/error.log", "fail2ban.log", "php/php8.5-fpm.log"} {
		if !validIdentifier(identifier) {
			t.Fatalf("valid identifier rejected: %q", identifier)
		}
	}
	for _, identifier := range []string{"", "/var/log/auth.log", "../auth.log", "apache2//error.log", "apache2/error log"} {
		if validIdentifier(identifier) {
			t.Fatalf("invalid identifier accepted: %q", identifier)
		}
	}
	for _, filename := range []string{"error.log.1", "error.log.2.gz", "error.log.old", "error.log.zst"} {
		if !excludedFilename(filename) {
			t.Fatalf("rotated file accepted: %q", filename)
		}
	}
}

func TestLogPolicyAcceptsOnlyVarLogPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs")
	content := "directory=/var/log/httpd\ndirectory=/var/log/httpd\nfile=/var/log/messages\nfile=/etc/shadow\ndirectory=/var/log/../etc\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	directories, files := loadPolicy(path)
	if len(directories) != 1 || directories[0] != "/var/log/httpd" || len(files) != 1 || files[0] != "/var/log/messages" {
		t.Fatalf("directories=%#v files=%#v", directories, files)
	}
}

func writeLog(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0640); err != nil {
		t.Fatal(err)
	}
}
