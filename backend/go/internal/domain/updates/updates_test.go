package updates

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
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

func fixture(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestParseAPTUpdates(t *testing.T) {
	updates := parseAPTUpdates(fixture(t, "apt-list-upgradable.txt"))
	if len(updates) != 3 || updates[0].Name != "libc6" || !updates[0].Security || updates[2].Security {
		t.Fatalf("updates = %#v", updates)
	}
}

func TestInfoAndList(t *testing.T) {
	directory := t.TempDir()
	apt := filepath.Join(directory, "apt")
	if err := os.WriteFile(apt, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	lists := filepath.Join(directory, "lists")
	if err := os.Mkdir(lists, 0o755); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(lists, "index")
	if err := os.WriteFile(index, []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 8, 15, 10, 11, 12, 0, time.UTC)
	if err := os.Chtimes(index, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{apt + " list --upgradable": {fixture(t, "apt-list-upgradable.txt"), 0}}
	backend := &Backend{runner: runner, apt: apt, aptLists: lists, rebootFile: filepath.Join(directory, "missing")}
	handler := New(backend)
	info := handler.Handle(context.Background(), "info", nil)
	if !info.Response.Success || (*info.Response.Data)["update_count"] != 3 || (*info.Response.Data)["security_update_count"] != 2 || (*info.Response.Data)["last_refresh"] != "2026-08-15T10:11:12Z" {
		t.Fatalf("info = %#v", info)
	}
	list := handler.Handle(context.Background(), "list", nil)
	if !list.Response.Success || (*list.Response.Data)["count"] != 3 {
		t.Fatalf("list = %#v", list)
	}
}

func TestDNFInfoAndList(t *testing.T) {
	directory := t.TempDir()
	dnf := filepath.Join(directory, "dnf")
	rpm := filepath.Join(directory, "rpm")
	for _, command := range []string{dnf, rpm} {
		if err := os.WriteFile(command, []byte("fixture"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cache := filepath.Join(directory, "cache")
	if err := os.Mkdir(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{
		dnf + " --quiet check-update": {"openssl.x86_64 3.2.2-1.el9 updates\nzlib.x86_64 1.3.1-2.el9 updates", 100},
		rpm + " -q --qf %{NAME}\\t%{ARCH}\\t%{EVR}\\n -- openssl.x86_64 zlib.x86_64": {"openssl\tx86_64\t3.2.1-1.el9\nzlib\tx86_64\t1.3-1.el9", 0},
		dnf + " --quiet updateinfo list --security --available":                      {"RHSA-2026:0001 Important/Sec. openssl-3.2.2-1.el9.x86_64", 0},
	}
	backend := &Backend{runner: runner, dnf: dnf, rpm: rpm, dnfCache: cache, rebootFile: filepath.Join(directory, "missing")}
	info := New(backend).Handle(context.Background(), "info", nil)
	if !info.Response.Success || (*info.Response.Data)["backend"] != "dnf" || (*info.Response.Data)["update_count"] != 2 || (*info.Response.Data)["security_update_count"] != 1 {
		t.Fatalf("dnf info = %#v", info)
	}
	list := New(backend).Handle(context.Background(), "list", nil)
	updates := (*list.Response.Data)["updates"].([]Update)
	if len(updates) != 2 || updates[0].InstalledVersion != "3.2.1-1.el9" || !updates[0].Security || updates[1].Security {
		t.Fatalf("dnf updates = %#v", updates)
	}
}

func TestFirmware(t *testing.T) {
	directory := t.TempDir()
	fwupd := filepath.Join(directory, "fwupdmgr")
	if err := os.WriteFile(fwupd, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{
		fwupd + " --version":           {"compile org.freedesktop.fwupd 2.0\nruntime org.freedesktop.fwupd 1.9.20", 0},
		fwupd + " get-upgrades --json": {fixture(t, "fwupd-upgrades.json"), 0},
	}
	reply := New(&Backend{runner: runner, fwupd: fwupd}).Handle(context.Background(), "firmware", nil)
	data := *reply.Response.Data
	if !reply.Response.Success || data["version"] != "1.9.20" || data["device_count"] != 1 || data["reboot_required"] != true {
		t.Fatalf("firmware = %#v", reply)
	}
	updates := data["updates"].([]FirmwareUpdate)
	if len(updates[0].Issues) != 1 || updates[0].CandidateVersion != "1.1" {
		t.Fatalf("updates = %#v", updates)
	}
}

func TestFirmwareUnavailableAndInvalid(t *testing.T) {
	unavailable := New(&Backend{fwupd: filepath.Join(t.TempDir(), "missing")}).Handle(context.Background(), "firmware", nil)
	if !unavailable.Response.Success || (*unavailable.Response.Data)["available"] != false {
		t.Fatalf("unavailable = %#v", unavailable)
	}
	directory := t.TempDir()
	fwupd := filepath.Join(directory, "fwupdmgr")
	if err := os.WriteFile(fwupd, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{fwupd + " get-upgrades --json": {"not-json", 0}}
	invalid := New(&Backend{runner: runner, fwupd: fwupd}).Handle(context.Background(), "firmware", nil)
	if invalid.Response.Error == nil || invalid.Response.Error.Code != "INVALID_FIRMWARE_UPDATES_RESPONSE" || invalid.ExitCode != 3 {
		t.Fatalf("invalid = %#v", invalid)
	}
}

func TestHandlerErrors(t *testing.T) {
	handler := New(&Backend{})
	for _, test := range []struct {
		command string
		args    []string
		code    string
	}{{"", nil, "MISSING_COMMAND"}, {"unknown", nil, "COMMAND_NOT_FOUND"}, {"info", []string{"extra"}, "INVALID_ARGUMENT_COUNT"}} {
		reply := handler.Handle(context.Background(), test.command, test.args)
		if reply.Response.Error == nil || reply.Response.Error.Code != test.code {
			t.Fatalf("reply = %#v", reply)
		}
	}
}

func TestUpgradeLifecycle(t *testing.T) {
	directory := t.TempDir()
	apt := filepath.Join(directory, "apt")
	aptGet := filepath.Join(directory, "apt-get")
	systemctl := filepath.Join(directory, "systemctl")
	for _, command := range []string{apt, systemctl} {
		if err := os.WriteFile(command, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(aptGet, []byte("#!/bin/sh\nprintf 'Running %s\\n' \"$*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(directory, "updates")
	if err := os.MkdirAll(filepath.Join(state, "jobs"), 0o750); err != nil {
		t.Fatal(err)
	}
	backend := &Backend{runner: fakeRunner{}, apt: apt, aptGet: aptGet, systemctl: systemctl, updateState: state}
	handler := New(backend)
	started := handler.Handle(context.Background(), "upgrade-start", nil)
	if !started.Response.Success {
		t.Fatalf("start = %#v", started)
	}
	jobID, ok := (*started.Response.Data)["job_id"].(string)
	if !ok || len(jobID) != 32 {
		t.Fatalf("job id = %#v", (*started.Response.Data)["job_id"])
	}
	repeated := handler.Handle(context.Background(), "upgrade-start", nil)
	if !repeated.Response.Success || (*repeated.Response.Data)["job_id"] != jobID || (*repeated.Response.Data)["already_running"] != true {
		t.Fatalf("repeated start = %#v", repeated)
	}

	if err := runUpdater(context.Background(), jobID, upgradeStore{root: state}, updaterCommands{aptGet: aptGet}); err != nil {
		t.Fatal(err)
	}
	// A fresh backend instance must recover the persisted result after a restart.
	status := New(&Backend{updateState: state}).Handle(context.Background(), "upgrade-status", []string{jobID})
	if !status.Response.Success {
		t.Fatalf("status = %#v", status)
	}
	data := *status.Response.Data
	lines := data["lines"].([]string)
	if data["status"] != "completed" || len(lines) < 6 || data["exit_code"] == nil {
		t.Fatalf("completed data = %#v", data)
	}
}

func TestUpgradeStatusRejectsInvalidJobID(t *testing.T) {
	reply := New(&Backend{}).Handle(context.Background(), "upgrade-status", []string{"invalid"})
	if reply.Response.Error == nil || reply.Response.Error.Code != "INVALID_JOB_ID" {
		t.Fatalf("reply = %#v", reply)
	}
}

func TestRebootNowAndDelayed(t *testing.T) {
	directory := t.TempDir()
	systemctl := filepath.Join(directory, "systemctl")
	systemdRun := filepath.Join(directory, "systemd-run")
	for _, command := range []string{systemctl, systemdRun} {
		if err := os.WriteFile(command, []byte("fixture"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runner := fakeRunner{
		systemctl + " reboot --no-block": {"", 0},
		systemdRun + " --unit=aegisadmin-reboot --collect --on-active=15m " + systemctl + " reboot --no-block": {"", 0},
	}
	handler := New(&Backend{runner: runner, systemctl: systemctl, systemdRun: systemdRun, updateState: directory})
	for _, delay := range []string{"0", "15"} {
		reply := handler.Handle(context.Background(), "reboot", []string{delay})
		if !reply.Response.Success || (*reply.Response.Data)["scheduled"] != true {
			t.Fatalf("delay %s: %#v", delay, reply)
		}
	}
	invalid := handler.Handle(context.Background(), "reboot", []string{"10"})
	if invalid.Response.Error == nil || invalid.Response.Error.Code != "INVALID_REBOOT_DELAY" {
		t.Fatalf("invalid delay: %#v", invalid)
	}
}

func TestRebootIsRefusedDuringUpgrade(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "updates")
	if err := os.Mkdir(directory, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "jobs"), 0o750); err != nil {
		t.Fatal(err)
	}
	id := "0123456789abcdef0123456789abcdef"
	store := upgradeStore{root: directory}
	if err := store.create(upgradeJob{ID: id, Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	systemctl := filepath.Join(directory, "systemctl")
	if err := os.WriteFile(systemctl, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	reply := New(&Backend{runner: fakeRunner{}, systemctl: systemctl, updateState: directory}).Handle(context.Background(), "reboot", []string{"0"})
	if reply.Response.Error == nil || reply.Response.Error.Code != "UPDATE_IN_PROGRESS" {
		t.Fatalf("reply=%#v", reply)
	}
}

func TestUpdaterStopsWhenMetadataRefreshFails(t *testing.T) {
	directory := t.TempDir()
	state := filepath.Join(directory, "updates")
	if err := os.MkdirAll(filepath.Join(state, "jobs"), 0o750); err != nil {
		t.Fatal(err)
	}
	id := "0123456789abcdef0123456789abcdef"
	store := upgradeStore{root: state}
	if err := store.create(upgradeJob{ID: id, Status: "pending", Backend: "apt", Lines: []string{}, StartedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	aptGet := filepath.Join(directory, "apt-get")
	if err := os.WriteFile(aptGet, []byte("#!/bin/sh\necho refresh-failed\nexit 42\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runUpdater(context.Background(), id, store, updaterCommands{aptGet: aptGet}); err == nil {
		t.Fatal("failed metadata refresh accepted")
	}
	job, err := store.read(id)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "failed" || job.ExitCode == nil || *job.ExitCode != 42 || job.FinishedAt == nil {
		t.Fatalf("failed job = %#v", job)
	}
}

func TestComposerProjectDiscovery(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "example")
	documentRoot := filepath.Join(project, "public")
	if err := os.MkdirAll(documentRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"composer.json", "composer.lock"} {
		if err := os.WriteFile(filepath.Join(project, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, ok := findComposerProject(documentRoot, []string{root}, 2)
	if !ok || found != project {
		t.Fatalf("project = %q, ok = %v", found, ok)
	}
}

func TestComposerDocumentRoot(t *testing.T) {
	configuration := filepath.Join(t.TempDir(), "site.conf")
	content := "<VirtualHost *:80>\nServerName example.test\nDocumentRoot /var/www/example/public\n</VirtualHost>\n"
	if err := os.WriteFile(configuration, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	root, ok := composerDocumentRoot(configuration, 1)
	if !ok || root != "/var/www/example/public" {
		t.Fatalf("document root = %q, ok = %v", root, ok)
	}
}

func TestParseComposerOutdated(t *testing.T) {
	payload := []byte("An optional Nextcloud plugin was skipped.\n" + `{"installed":[{"name":"psr/log","version":"3.0.0","latest":"3.0.2","latest-status":"semver-safe-update","direct-dependency":true},{"name":"psr/container","version":"2.0.2","latest":"2.0.2","latest-status":"up-to-date","direct-dependency":true}]}`)
	packages, err := parseComposerOutdated(payload)
	if err != nil || len(packages) != 1 || packages[0].Name != "psr/log" || !packages[0].Direct {
		t.Fatalf("packages = %#v, err = %v", packages, err)
	}
	empty, err := parseComposerOutdated([]byte("[]\n"))
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty packages = %#v, err = %v", empty, err)
	}
}

func TestParseComposerAudit(t *testing.T) {
	payload := []byte(`{"advisories":{"vendor/package":[{"advisoryId":"PKSA-test","packageName":"vendor/package","title":"Faille de test","link":"https://example.test/advisory","affectedVersions":"<1.2.3"}]}}`)
	advisories, err := parseComposerAudit(payload)
	if err != nil || len(advisories) != 1 {
		t.Fatalf("advisories = %#v, err = %v", advisories, err)
	}
	if advisories[0].ID != "PKSA-test" || advisories[0].Package != "vendor/package" || advisories[0].AffectedVersions != "<1.2.3" {
		t.Fatalf("advisory = %#v", advisories[0])
	}
	empty, err := parseComposerAudit([]byte(" [] "))
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty advisories = %#v, err = %v", empty, err)
	}
	empty, err = parseComposerAudit([]byte(`{"advisories":[],"abandoned":[]}`))
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty Composer audit = %#v, err = %v", empty, err)
	}
	empty, err = parseComposerAudit([]byte(`{"advisories":[],$prefix}`))
	if err != nil || len(empty) != 0 {
		t.Fatalf("noisy empty Composer audit = %#v, err = %v", empty, err)
	}
}

func TestComposerCompatibilityAndFailureMessages(t *testing.T) {
	if !composerRejectsLocked([]byte(`The "--locked" option does not exist.`)) {
		t.Fatal("the legacy Composer option error must be recognized")
	}
	if !composerRejectsAbandonedMode([]byte(`The "--abandoned" option does not exist.`)) {
		t.Fatal("the legacy Composer abandoned option error must be recognized")
	}
	message := composerFailureMessage([]byte("Permission denied"), &exec.ExitError{}, false, uint32(os.Getuid()), uint32(os.Getgid()))
	if message != "Composer ne peut pas lire le projet avec l’identité non-root configurée." {
		t.Fatalf("message = %q", message)
	}
	message = composerFailureMessage(nil, context.DeadlineExceeded, true, uint32(os.Getuid()), uint32(os.Getgid()))
	if message != "La vérification Composer a dépassé le délai de 90 secondes." {
		t.Fatalf("message = %q", message)
	}
}

func TestComposerCredentialsDoNotRequireSetgroups(t *testing.T) {
	command := exec.Command("/usr/bin/true")
	command.SysProcAttr = composerCredentials(uint32(os.Getuid()), uint32(os.Getgid()))
	if err := command.Run(); err != nil {
		t.Fatalf("start child with current identity: %v", err)
	}
}

func TestComposerInvocationDisablesPCREJIT(t *testing.T) {
	executable, arguments := composerInvocation(
		"/usr/bin/php",
		"/usr/local/bin/composer",
		[]string{"--no-interaction", "outdated"},
	)
	if executable != "/usr/bin/php" {
		t.Fatalf("executable = %q", executable)
	}
	wanted := []string{"-d", "pcre.jit=0", "/usr/local/bin/composer", "--no-interaction", "outdated"}
	if !reflect.DeepEqual(arguments, wanted) {
		t.Fatalf("arguments = %#v", arguments)
	}
}

func TestComposerEnvironmentRewritesPublicGitHubSSHURLs(t *testing.T) {
	environment := composerEnvironment("/tmp/composer-test")
	for _, wanted := range []string{
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=url.https://github.com/.insteadOf",
		"GIT_CONFIG_VALUE_0=git@github.com:",
		"GIT_CONFIG_KEY_1=url.https://github.com/.insteadOf",
		"GIT_CONFIG_VALUE_1=ssh://git@github.com/",
	} {
		if !slices.Contains(environment, wanted) {
			t.Fatalf("missing environment entry %q in %#v", wanted, environment)
		}
	}
}
