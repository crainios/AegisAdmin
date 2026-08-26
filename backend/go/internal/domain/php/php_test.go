package php

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureRunner struct {
	units      string
	properties map[string]string
	info       string
	extensions string
	config     string
	restartOK  bool
	calls      []string
}

func (f *fixtureRunner) Run(_ context.Context, environment map[string]string, name string, arguments ...string) (commandResult, *Error) {
	f.calls = append(f.calls, name+" "+strings.Join(arguments, " "))
	if name == systemctlCommand && len(arguments) > 0 && arguments[0] == "list-unit-files" {
		return commandResult{Output: f.units, OK: true}, nil
	}
	if name == systemctlCommand && len(arguments) > 0 && arguments[0] == "show" {
		units := arguments[8:]
		var output strings.Builder
		for _, unit := range units {
			output.WriteString("Id=" + unit + "\n")
			output.WriteString("Names=" + unit + "\n")
			for _, property := range []string{"LoadState", "ActiveState", "SubState", "UnitFileState"} {
				output.WriteString(property + "=" + f.properties[unit+":"+property] + "\n")
			}
			output.WriteString("\n")
		}
		return commandResult{Output: output.String(), OK: true}, nil
	}
	if name == systemdRunCommand {
		return commandResult{OK: f.restartOK}, nil
	}
	if len(arguments) >= 2 && arguments[len(arguments)-2] == "-r" {
		switch arguments[len(arguments)-1] {
		case versionSource:
			return commandResult{Output: "8.5", OK: true}, nil
		case infoSource:
			return commandResult{Output: f.info, OK: true}, nil
		case configurationSource:
			if strings.Contains(name, "php8.5") && environment["PHP_INI_SCAN_DIR"] != "/etc/php/8.5/fpm/conf.d" {
				return commandResult{}, domainError(10, "TEST_ENVIRONMENT", "invalid environment", nil)
			}
			return commandResult{Output: f.config, OK: true}, nil
		case extensionsSource:
			return commandResult{Output: f.extensions, OK: true}, nil
		}
	}
	return commandResult{}, domainError(10, "TEST_UNEXPECTED_COMMAND", name, nil)
}

func TestDiscoversGenericFPMInstance(t *testing.T) {
	runner := standardRunner(t)
	runner.units = "php-fpm.service enabled enabled"
	runner.properties = map[string]string{
		"php-fpm.service:LoadState": "loaded", "php-fpm.service:ActiveState": "active",
		"php-fpm.service:SubState": "running", "php-fpm.service:UnitFileState": "enabled",
	}
	backend := testBackend(t, runner)
	instances, err := backend.instances(context.Background())
	if err != nil || len(instances) != 1 {
		t.Fatalf("unexpected generic instances: %#v, %#v", instances, err)
	}
	instance := instances[0]
	if instance.ID != "fpm-8.5" || instance.Service != "php-fpm" || instance.Unit != "php-fpm.service" || instance.binary != filepath.Join(backend.binDir, "php") || instance.debianLayout {
		t.Fatalf("unexpected generic instance: %#v", instance)
	}
}

func TestHandlerValidation(t *testing.T) {
	backend := testBackend(t, &fixtureRunner{info: readFixture(t, "php-info.json")})
	reply := New(backend).Handle(context.Background(), "info", nil)
	if reply.ExitCode != 0 || reply.Response.Data == nil || (*reply.Response.Data)["runtime"] != "cli" {
		t.Fatalf("unexpected info: %#v", reply)
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
		{"configuration", nil, 2, "INVALID_ARGUMENT_COUNT"},
	}
	for _, test := range tests {
		result := New(backend).Handle(context.Background(), test.command, test.args)
		if result.ExitCode != test.exit || result.Response.Error == nil || result.Response.Error.Code != test.code {
			t.Fatalf("unexpected reply: %#v", result)
		}
	}
}

func TestDiscoversFPMInstancesFromFixture(t *testing.T) {
	runner := standardRunner(t)
	backend := testBackend(t, runner)
	instances, err := backend.instances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 2 || instances[0].ID != "fpm-8.5" || instances[1].ID != "fpm-8.4" {
		t.Fatalf("unexpected instances: %#v", instances)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected two batched systemctl calls, got %#v", runner.calls)
	}
	if !instances[0].Exists || !instances[0].Active || !instances[0].Enabled || instances[0].State != "running" {
		t.Fatalf("unexpected 8.5 instance: %#v", instances[0])
	}
	if instances[1].Active || instances[1].Enabled || instances[1].State != "inactive" {
		t.Fatalf("unexpected 8.4 instance: %#v", instances[1])
	}
}

func TestRuntimeQueriesAndRestart(t *testing.T) {
	runner := standardRunner(t)
	backend := testBackend(t, runner)
	configuration, err := backend.configuration(context.Background(), "fpm-8.5")
	if err != nil || configuration["runtime"] != "fpm-8.5" {
		t.Fatalf("unexpected configuration: %#v, %#v", configuration, err)
	}
	directives := configuration["directives"].(map[string]any)
	if len(directives) != len(importantDirectives) || directives["memory_limit"] != "128M" {
		t.Fatalf("unexpected directives: %#v", directives)
	}
	extensions, err := backend.extensions(context.Background(), "cli")
	if err != nil || len(extensions["extensions"].([]any)) != 4 {
		t.Fatalf("unexpected extensions: %#v, %#v", extensions, err)
	}
	restart, err := backend.restart(context.Background(), "fpm-8.5")
	if err != nil || restart["result"] != "scheduled" || restart["delay_seconds"] != 5 {
		t.Fatalf("unexpected restart: %#v, %#v", restart, err)
	}
}

func TestRuntimeErrors(t *testing.T) {
	backend := testBackend(t, standardRunner(t))
	tests := []struct {
		identifier string
		code       string
	}{
		{"invalid", "INVALID_PHP_RUNTIME_ID"},
		{"fpm-9.9", "PHP_RUNTIME_NOT_FOUND"},
	}
	for _, test := range tests {
		_, err := backend.requireRuntime(context.Background(), test.identifier)
		if err == nil || err.Code != test.code {
			t.Fatalf("unexpected error for %s: %#v", test.identifier, err)
		}
	}
	if _, err := backend.restart(context.Background(), "cli"); err == nil || err.Code != "PHP_RUNTIME_NOT_RESTARTABLE" {
		t.Fatalf("unexpected CLI restart error: %#v", err)
	}
}

func standardRunner(t *testing.T) *fixtureRunner {
	t.Helper()
	directives := make([]string, 0, len(importantDirectives))
	for _, name := range importantDirectives {
		directives = append(directives, `"`+name+`":"128M"`)
	}
	return &fixtureRunner{
		units: readFixture(t, "fpm-units.txt"), info: readFixture(t, "php-info.json"),
		extensions: readFixture(t, "php-extensions.json"),
		config:     `{"ini_file":"/etc/php/8.5/fpm/php.ini","scan_dir":"/etc/php/8.5/fpm/conf.d","directives":{` + strings.Join(directives, ",") + `}}`,
		restartOK:  true,
		properties: map[string]string{
			"php8.5-fpm.service:LoadState": "loaded", "php8.5-fpm.service:ActiveState": "active",
			"php8.5-fpm.service:SubState": "running", "php8.5-fpm.service:UnitFileState": "enabled",
			"php8.4-fpm.service:LoadState": "loaded", "php8.4-fpm.service:ActiveState": "inactive",
			"php8.4-fpm.service:SubState": "", "php8.4-fpm.service:UnitFileState": "disabled",
		},
	}
}

func testBackend(t *testing.T, runner commandRunner) *Backend {
	t.Helper()
	binDir := t.TempDir()
	for _, name := range []string{"php", "php8.4", "php8.5"} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("fixture"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return &Backend{runner: runner, binDir: binDir}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(content))
}
