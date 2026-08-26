package cron

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeRunner map[string]struct {
	output string
	status int
}

func (runner fakeRunner) Run(_ context.Context, name string, args ...string) (string, int) {
	key := name
	for _, argument := range args {
		key += " " + argument
	}
	result := runner[key]
	return result.output, result.status
}
func (runner fakeRunner) RunInput(ctx context.Context, _ string, name string, args ...string) (string, int) {
	return runner.Run(ctx, name, args...)
}

func copyFixture(t *testing.T, name, destination string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestJobsParsesSystemAndManagedTasks(t *testing.T) {
	directory := t.TempDir()
	system := filepath.Join(directory, "crontab")
	passwd := filepath.Join(directory, "passwd")
	copyFixture(t, "system-crontab", system)
	if err := os.WriteFile(passwd, []byte("alice:x:1000:1000::/home/alice:/bin/bash\nwww-data:x:33:33::/var/www:/usr/sbin/nologin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &Backend{paths: Paths{SystemCrontab: system, CronDirectory: filepath.Join(directory, "cron.d"), SpoolDirectory: filepath.Join(directory, "spool"), PasswdFile: passwd, LoginDefsFile: filepath.Join(directory, "login.defs"), AllowedUsersFile: filepath.Join(directory, "cron-users")}}
	jobs, err := backend.jobs()
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || jobs[0].User != "root" || jobs[1].Schedule != "@reboot" {
		t.Fatalf("jobs = %#v", jobs)
	}
	userContent, err := os.ReadFile(filepath.Join("testdata", "user-crontab"))
	if err != nil {
		t.Fatal(err)
	}
	p := &jobParser{backend: backend, jobs: []Job{}, periods: map[string]string{}}
	userFile := filepath.Join(directory, "alice")
	if err := os.WriteFile(userFile, userContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := p.parseCrontab(userFile, "user", "alice", true); err != nil {
		t.Fatal(err)
	}
	if len(p.jobs) != 3 || p.jobs[0].Managed || !p.jobs[1].Managed || p.jobs[1].Command != "/usr/local/bin/managed  two-spaces" || p.jobs[2].Enabled {
		t.Fatalf("managed jobs = %#v", p.jobs)
	}
}

func TestLegacyTaskIDDoesNotDependOnCommentLineNumbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy")
	if err := os.WriteFile(path, []byte("# spool metadata\n# another comment\n*/5 * * * * /usr/bin/true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	parser := &jobParser{jobs: []Job{}, editable: map[string]bool{"deploy": true}, periods: map[string]string{}}
	if err := parser.parseCrontab(path, "user", "deploy", true); err != nil {
		t.Fatal(err)
	}
	records, err := parseRecords("deploy", []string{"*/5 * * * * /usr/bin/true"})
	if err != nil || len(parser.jobs) != 1 || len(records) != 1 || parser.jobs[0].ID != records[0].id {
		t.Fatalf("jobs=%#v records=%#v err=%v", parser.jobs, records, err)
	}
}

func TestUsersDiscoversHumansAndConfiguredServiceAccounts(t *testing.T) {
	directory := t.TempDir()
	passwd := filepath.Join(directory, "passwd")
	loginDefs := filepath.Join(directory, "login.defs")
	allowed := filepath.Join(directory, "cron-users")
	denied := filepath.Join(directory, "cron-users-deny")
	if err := os.WriteFile(passwd, []byte("root:x:0:0::/root:/bin/bash\nwww-data:x:33:33::/var/www:/usr/sbin/nologin\nservice:x:998:998::/srv/service:/usr/sbin/nologin\nalice:x:1001:1001::/home/alice:/bin/bash\nrsync:x:1002:1002::/home/rsync:/bin/sh\nnobody:x:65534:65534::/nonexistent:/usr/sbin/nologin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loginDefs, []byte("UID_MIN 1000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(allowed, []byte("# Comptes techniques\nwww-data\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(denied, []byte("# Comptes exclus\nrsync\nwww-data\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &Backend{paths: Paths{PasswdFile: passwd, LoginDefsFile: loginDefs, AllowedUsersFile: allowed, DeniedUsersFile: denied}}
	users, err := backend.users()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Name != "alice" || users[0].System {
		t.Fatalf("users = %#v", users)
	}
	if backend.editable("root") || backend.editable("service") || backend.editable("rsync") || backend.editable("www-data") || !backend.editable("alice") {
		t.Fatal("editable user policy mismatch")
	}
}

func TestParseRecordsAndValidation(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("testdata", "user-crontab"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := parseRecords("alice", splitContent(string(content)))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || !legacyID.MatchString(records[0].id) || records[2].enabled {
		t.Fatalf("records = %#v", records)
	}
	if !validTaskInput("*/5 * * * *", "echo ok") || !validTaskInput("@daily", "echo ok") || validTaskInput("@sometimes", "echo") || validTaskInput("* * * *", "echo") {
		t.Fatal("task validation mismatch")
	}
}

func TestInfoStatusAndErrors(t *testing.T) {
	directory := t.TempDir()
	executablePath := filepath.Join(directory, "tool")
	if err := os.WriteFile(executablePath, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := Paths{DpkgQuery: executablePath, Systemctl: executablePath, Crontab: executablePath, Anacron: filepath.Join(directory, "missing")}
	runner := fakeRunner{
		executablePath + " --show --showformat=${Status} -- cron":             {"install ok installed", 0},
		executablePath + " --show --showformat=${Version} -- cron":            {"3.0pl1-184", 0},
		executablePath + " show --property=LoadState --value -- cron.service": {"loaded", 0},
		executablePath + " is-active --quiet -- cron.service":                 {"", 0},
		executablePath + " is-enabled --quiet -- cron.service":                {"", 1},
	}
	backend := &Backend{runner: runner, paths: paths, now: time.Now, packageName: service, service: service, unit: unit}
	handler := New(backend)
	info := handler.Handle(context.Background(), "info", nil)
	if !info.Response.Success || (*info.Response.Data)["version"] != "3.0pl1-184" {
		t.Fatalf("info = %#v", info)
	}
	status := handler.Handle(context.Background(), "status", nil)
	if !status.Response.Success || (*status.Response.Data)["active"] != true || (*status.Response.Data)["enabled"] != false {
		t.Fatalf("status = %#v", status)
	}
	for _, item := range []struct {
		command string
		args    []string
		code    string
	}{{"", nil, "MISSING_COMMAND"}, {"unknown", nil, "COMMAND_NOT_FOUND"}, {"info", []string{"x"}, "INVALID_ARGUMENT_COUNT"}} {
		reply := handler.Handle(context.Background(), item.command, item.args)
		if reply.Response.Error == nil || reply.Response.Error.Code != item.code {
			t.Fatalf("reply = %#v", reply)
		}
	}
}

func TestProfileSelectsCronieLayout(t *testing.T) {
	directory := t.TempDir()
	profile := filepath.Join(directory, "cron-profile")
	if err := os.WriteFile(profile, []byte("package=cronie\nunit=crond.service\nspool_directory=/var/spool/cron\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &Backend{
		packageName: service,
		service:     service,
		unit:        unit,
		paths: Paths{
			SpoolDirectory: "/var/spool/cron/crontabs",
		},
	}
	backend.loadProfile(profile)
	if backend.packageName != "cronie" || backend.service != "crond" || backend.unit != "crond.service" || backend.paths.SpoolDirectory != "/var/spool/cron" {
		t.Fatalf("backend = %#v", backend)
	}
}

func TestPackageVersionUsesRPM(t *testing.T) {
	directory := t.TempDir()
	rpm := filepath.Join(directory, "rpm")
	if err := os.WriteFile(rpm, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	backend := &Backend{
		runner: fakeRunner{
			rpm + " -q --qf %{EVR} cronie": {"1.7.2-1.el9", 0},
		},
		paths:       Paths{RPM: rpm},
		packageName: "cronie",
	}
	if version := backend.packageVersion(context.Background()); version != "1.7.2-1.el9" {
		t.Fatalf("version = %q", version)
	}
}

func TestRunResultValidation(t *testing.T) {
	id := "0123456789abcdef0123456789abcdef"
	if _, failure := (&Backend{}).runResult("invalid"); failure == nil || failure.Response.Error.Code != "INVALID_CRON_EXECUTION_ID" {
		t.Fatalf("failure = %#v", failure)
	}
	result := ExecutionResult{id, "finished", intPointer(0), false, false, 12, "ok", ""}
	if !validResult(result) {
		t.Fatal("valid result rejected")
	}
	result.Status = "unknown"
	if validResult(result) {
		t.Fatal("invalid result accepted")
	}
}
func intPointer(value int) *int { return &value }

func TestUnsupportedCronValidation(t *testing.T) {
	for _, message := range []string{"crontab: invalid option -- 'n'", "illegal option -- n", "Usage: crontab [-u user] file"} {
		if !unsupportedCronValidation(message) {
			t.Fatalf("unsupported validation not detected: %q", message)
		}
	}
	if unsupportedCronValidation("bad minute") {
		t.Fatal("a genuine syntax error must not disable validation")
	}
}
