package apache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakeBackend struct {
	data map[string]any
	err  *Error
	seen string
	arg  string
}

func (f *fakeBackend) Execute(_ context.Context, command, argument string) (map[string]any, *Error) {
	f.seen, f.arg = command, argument
	return f.data, f.err
}

type fakeRunner struct {
	results map[string]commandResult
	errors  map[string]*Error
	calls   []string
	mu      sync.Mutex
}

func (f *fakeRunner) Run(_ context.Context, name string, arguments ...string) (commandResult, *Error) {
	key := name + " " + strings.Join(arguments, " ")
	f.mu.Lock()
	f.calls = append(f.calls, key)
	f.mu.Unlock()
	if err := f.errors[key]; err != nil {
		return commandResult{}, err
	}
	result, exists := f.results[key]
	if !exists {
		return commandResult{}, domainError(10, "TEST_UNEXPECTED_COMMAND", key, nil)
	}
	return result, nil
}

func TestOverviewCollectsApacheState(t *testing.T) {
	runner := &fakeRunner{results: map[string]commandResult{
		apacheCommand + " -v":            {Output: readFixture(t, "apache-version.txt"), OK: true},
		apachectlCommand + " configtest": {Output: "Syntax OK", OK: true},
		apachectlCommand + " -S":         {Output: "", OK: true},
		apachectlCommand + " -M":         {Output: readFixture(t, "apache-modules.txt"), OK: true},
	}}
	backend := testBackend(t, runner)
	data, err := backend.overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 6 || data["installed"] != true || data["info"].(map[string]any)["version"] != "Apache/2.4.64 (Debian)" {
		t.Fatalf("unexpected overview: %#v", data)
	}
	if data["configtest"].(map[string]any)["valid"] != true {
		t.Fatalf("unexpected configtest: %#v", data["configtest"])
	}
	if data["sites"].(map[string]any)["count"] != 0 {
		t.Fatalf("unexpected sites: %#v", data["sites"])
	}
	if len(runner.calls) != 4 {
		t.Fatalf("unexpected command count: %d", len(runner.calls))
	}
}

func TestVhostsPreservesSourceOrderForEqualSortKeys(t *testing.T) {
	config := filepath.Join(t.TempDir(), "site.conf")
	if err := os.WriteFile(config, []byte("<VirtualHost *:80>\n</VirtualHost>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := "default server invalid.local (" + config + ":12)\n" +
		"default server invalid.local (" + config + ":1)\n"
	runner := &fakeRunner{results: map[string]commandResult{
		apachectlCommand + " -S": {Output: output, OK: true},
	}}
	backend := testBackend(t, runner)
	data, err := backend.vhosts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	vhosts := data["virtual_hosts"].([]map[string]any)
	if len(vhosts) != 2 || vhosts[0]["config_line"] != 12 || vhosts[1]["config_line"] != 1 {
		t.Fatalf("vhosts=%#v", vhosts)
	}
}

func TestHandlerContracts(t *testing.T) {
	backend := &fakeBackend{data: map[string]any{"valid": true, "message": "Syntax OK"}}
	reply := New(backend).Handle(context.Background(), "configtest", nil)
	if reply.ExitCode != 0 || !reply.Response.Success || backend.seen != "configtest" {
		t.Fatalf("unexpected reply: %#v", reply)
	}

	tests := []struct {
		command string
		args    []string
		exit    int
		code    string
	}{
		{"", nil, 2, "MISSING_COMMAND"},
		{"unknown", nil, 4, "COMMAND_NOT_FOUND"},
		{"info", []string{"extra"}, 2, "INVALID_ARGUMENT_COUNT"},
		{"site", nil, 2, "INVALID_ARGUMENT_COUNT"},
	}
	for _, test := range tests {
		result := New(&fakeBackend{}).Handle(context.Background(), test.command, test.args)
		if result.ExitCode != test.exit || result.Response.Error == nil || result.Response.Error.Code != test.code {
			t.Fatalf("unexpected error reply: %#v", result)
		}
	}
}

func TestInfoAndModulesFixtures(t *testing.T) {
	version := readFixture(t, "apache-version.txt")
	modules := readFixture(t, "apache-modules.txt")
	runner := &fakeRunner{results: map[string]commandResult{
		apacheCommand + " -v":    {Output: version, OK: true},
		apachectlCommand + " -M": {Output: modules, OK: true},
	}}
	backend := testBackend(t, runner)
	info, err := backend.info(context.Background())
	if err != nil || info["version"] != "Apache/2.4.64 (Debian)" || info["built"] != "2026-07-18T10:00:00" {
		t.Fatalf("unexpected info: %#v, %#v", info, err)
	}
	listing, err := backend.modules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	items := listing["modules"].([]map[string]any)
	if len(items) != 3 || items[0]["name"] != "core" || items[2]["name"] != "ssl" {
		t.Fatalf("unexpected modules: %#v", items)
	}
}

func TestSitesAndSiteFixture(t *testing.T) {
	backend := testBackend(t, &fakeRunner{})
	content := readFixture(t, "site.conf")
	path := filepath.Join(backend.sitesAvailable, "example.conf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, filepath.Join(backend.sitesEnabled, "example.conf")); err != nil {
		t.Fatal(err)
	}
	listing, domainErr := backend.sites()
	if domainErr != nil {
		t.Fatal(domainErr)
	}
	sites := listing["sites"].([]map[string]any)
	if listing["count"] != 1 || len(sites) != 1 || sites[0]["enabled"] != true {
		t.Fatalf("unexpected sites: %#v", listing)
	}
	if len(sites[0]["server_names"].([]string)) != 2 || len(sites[0]["ports"].([]int)) != 2 {
		t.Fatalf("unexpected parsed site: %#v", sites[0])
	}
	detail, domainErr := backend.site(sites[0]["config_id"].(string))
	if domainErr != nil || detail["content"] != content {
		t.Fatalf("unexpected site detail: %#v, %#v", detail, domainErr)
	}
}

func TestCreateSiteWithValidation(t *testing.T) {
	runner := &fakeRunner{results: map[string]commandResult{
		apachectlCommand + " configtest": {Output: "Syntax OK", OK: true},
	}}
	backend := testBackend(t, runner)
	configuration := "<VirtualHost *:80>\nServerName created.test\n</VirtualHost>"
	payload, _ := json.Marshal(map[string]any{
		"filename": "created.conf",
		"content":  base64.StdEncoding.EncodeToString([]byte(configuration)),
	})
	result, domainErr := backend.create(context.Background(), string(payload))
	if domainErr != nil || result["action"] != "create" || result["enabled"] != false {
		t.Fatalf("unexpected create result: %#v, %#v", result, domainErr)
	}
	written, err := os.ReadFile(filepath.Join(backend.sitesAvailable, "created.conf"))
	if err != nil || string(written) != configuration+"\n" {
		t.Fatalf("unexpected created content: %q, %v", written, err)
	}
}

func TestPayloadAndIdentifierValidation(t *testing.T) {
	if _, err := parsePayload(`[]`); err == nil || err.Code != "INVALID_APACHE_SITE_PAYLOAD" {
		t.Fatalf("unexpected payload error: %#v", err)
	}
	invalid := base64.StdEncoding.EncodeToString([]byte{'a', 0, 'b'})
	payload := sitePayload{Content: &invalid}
	if _, err := decodeContent(payload); err == nil || err.Code != "INVALID_APACHE_SITE_CONTENT" {
		t.Fatalf("unexpected content error: %#v", err)
	}
	backend := testBackend(t, &fakeRunner{})
	if _, err := backend.sitePath("invalid"); err == nil || err.Code != "INVALID_APACHE_SITE_ID" {
		t.Fatalf("unexpected identifier error: %#v", err)
	}
}

func TestBackendErrorDetailsArePreserved(t *testing.T) {
	backend := &fakeBackend{err: domainError(10, "FAILED", "failed", map[string]any{"output": "details"})}
	reply := New(backend).Handle(context.Background(), "info", nil)
	if reply.Response.Error == nil || reply.Response.Error.Details["output"] != "details" {
		t.Fatalf("unexpected response: %#v", reply)
	}
}

func testBackend(t *testing.T, runner commandRunner) *LinuxBackend {
	t.Helper()
	root := t.TempDir()
	available := filepath.Join(root, "etc", "apache2", "sites-available")
	enabled := filepath.Join(root, "etc", "apache2", "sites-enabled")
	backup := filepath.Join(root, "backups")
	for _, directory := range []string{available, enabled, backup} {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
	}
	return &LinuxBackend{
		runner: runner, service: "apache2", apacheBinary: apacheCommand,
		apacheControl: apachectlCommand, enableCommand: a2ensiteCommand,
		disableCommand: a2dissiteCommand, sitesAvailable: available,
		sitesEnabled: enabled, backupRoot: backup,
		apacheRoot: filepath.Join(root, "etc", "apache2"),
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(content))
}

func TestApacheProfileSupportsAlternativeLayouts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apache")
	content := "service=httpd\nbinary=/usr/sbin/httpd\ncontrol=/usr/sbin/apachectl\nconfig_root=/etc/httpd\nsites_available=/etc/httpd/conf.d\nsites_enabled=/etc/httpd/conf.d\nenable_command=relative\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	profile := loadApacheProfile(path)
	if profile.service != "httpd" || profile.binary != "/usr/sbin/httpd" || profile.configRoot != "/etc/httpd" || profile.sitesAvailable != "/etc/httpd/conf.d" {
		t.Fatalf("profile=%#v", profile)
	}
	if profile.enableCommand != a2ensiteCommand {
		t.Fatalf("relative command accepted: %q", profile.enableCommand)
	}
}
