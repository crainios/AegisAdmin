package cron

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

const (
	service = "cron"
	unit    = "cron.service"
)

type Runner interface {
	Run(context.Context, string, ...string) (string, int)
	RunInput(context.Context, string, string, ...string) (string, int)
}

type Paths struct {
	DpkgQuery, RPM, Systemctl, Anacron, Crontab, SystemdRun, Runner string
	Anacrontab, SystemCrontab, CronDirectory, SpoolDirectory        string
	BackupDirectory, ResultDirectory, LockFile                      string
	TempDirectory, PasswdFile, LoginDefsFile, AllowedUsersFile      string
	DeniedUsersFile                                                 string
	BackupTaskDirectory, BackupCronDirectory, BackupExecutable      string
}

type Backend struct {
	runner                     Runner
	paths                      Paths
	now                        func() time.Time
	packageName, service, unit string
}

type Handler struct{ backend *Backend }
type execRunner struct{}

var argumentCounts = map[string]int{
	"info": 0, "status": 0, "users": 0, "jobs": 0, "create": 3, "update": 4,
	"suspend": 2, "resume": 2, "delete": 2, "run": 2, "run-result": 1,
	"backup-create": 12,
	"backup-delete": 1, "backup-run": 1,
}

var runnerPath = "/usr/local/lib/aegisadmin-system/libexec/cron-runner.py"

func New(backend *Backend) *Handler { return &Handler{backend} }

func NewLinuxBackend() *Backend {
	b := &Backend{runner: execRunner{}, now: time.Now, packageName: service, service: service, unit: unit, paths: Paths{
		DpkgQuery: "/usr/bin/dpkg-query", Systemctl: "/usr/bin/systemctl",
		RPM:     "/usr/bin/rpm",
		Anacron: "/usr/sbin/anacron", Crontab: "/usr/bin/crontab",
		SystemdRun: "/usr/bin/systemd-run",
		Runner:     runnerPath,
		Anacrontab: "/etc/anacrontab", SystemCrontab: "/etc/crontab",
		CronDirectory: "/etc/cron.d", SpoolDirectory: "/var/spool/cron/crontabs",
		BackupDirectory: "/var/backups/aegisadmin-system/cron",
		ResultDirectory: "/run/aegisadmin-system/cron",
		LockFile:        "/run/lock/aegisadmin-cron.lock",
		TempDirectory:   "/run",
		PasswdFile:      "/etc/passwd", LoginDefsFile: "/etc/login.defs",
		AllowedUsersFile:    "/etc/aegisadmin-system/cron-users",
		DeniedUsersFile:     "/etc/aegisadmin-system/cron-users-deny",
		BackupTaskDirectory: "/etc/aegisadmin-system/backup-tasks",
		BackupCronDirectory: "/etc/cron.d", BackupExecutable: "/usr/libexec/aegisadmin/aegisadmin-backup",
	}}
	if !executable(b.paths.DpkgQuery) && executable(b.paths.RPM) {
		b.packageName, b.service, b.unit = "cronie", "crond", "crond.service"
		b.paths.SpoolDirectory = "/var/spool/cron"
	}
	b.loadProfile("/etc/aegisadmin-system/cron")
	return b
}

func (execRunner) Run(ctx context.Context, name string, args ...string) (string, int) {
	return runCommand(ctx, "", name, args...)
}
func (execRunner) RunInput(ctx context.Context, input, name string, args ...string) (string, int) {
	return runCommand(ctx, input, name, args...)
}
func runCommand(ctx context.Context, input, name string, args ...string) (string, int) {
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, name, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	text := strings.ToValidUTF8(string(out), "�")
	if !(strings.HasSuffix(name, "/crontab") && len(args) >= 1 && args[len(args)-1] == "-l") {
		text = strings.TrimSpace(text)
	}
	if err == nil {
		return text, 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return text, exitError.ExitCode()
	}
	return text, -1
}

func (h *Handler) Handle(ctx context.Context, command string, args []string) protocol.Reply {
	if command == "" {
		return fail(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine cron.")
	}
	expected, exists := argumentCounts[command]
	if !exists {
		return fail(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine cron.")
	}
	if len(args) != expected {
		return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
	}
	if failure := h.backend.dependencies(command); failure != nil {
		return *failure
	}
	data, failure := h.backend.execute(ctx, command, args)
	if failure != nil {
		return *failure
	}
	return protocol.Reply{Response: api.Success(data)}
}

func (b *Backend) dependencies(command string) *protocol.Reply {
	for _, dependency := range []string{b.paths.Systemctl} {
		if !executable(dependency) {
			return reply(3, "DEPENDENCY_NOT_FOUND", "Une dépendance nécessaire au domaine cron est introuvable.")
		}
	}
	if command == "status" {
		return nil
	}
	if command != "info" && command != "jobs" && !executable(b.paths.Crontab) {
		return reply(3, "DEPENDENCY_NOT_FOUND", "La dépendance crontab nécessaire à la gestion des tâches est introuvable.")
	}
	if command == "run" || command == "run-result" {
		if !executable(b.paths.SystemdRun) {
			return reply(3, "DEPENDENCY_NOT_FOUND", "La dépendance systemd-run nécessaire au test des tâches est introuvable.")
		}
		if !executable(b.paths.Runner) {
			return reply(3, "DEPENDENCY_NOT_FOUND", "Le lanceur contrôlé des tâches Cron est introuvable.")
		}
	}
	return nil
}

func (b *Backend) execute(ctx context.Context, command string, args []string) (map[string]any, *protocol.Reply) {
	if command != "status" && !executable(b.paths.Crontab) {
		return nil, reply(5, "CRON_NOT_INSTALLED", "Cron n’est pas installé sur ce serveur.")
	}
	switch command {
	case "info":
		version := b.packageVersion(ctx)
		if version == "" {
			return nil, reply(10, "CRON_VERSION_UNAVAILABLE", "La version de Cron n’a pas pu être déterminée.")
		}
		return map[string]any{"product": "Cron", "version": version, "service": b.service, "unit": b.unit, "anacron_available": executable(b.paths.Anacron) && regularFile(b.paths.Anacrontab)}, nil
	case "status":
		return b.status(ctx), nil
	case "jobs":
		jobs, err := b.jobs()
		if err != nil {
			return nil, reply(10, "CRON_JOBS_READ_FAILED", "Les tâches Cron n’ont pas pu être lues.")
		}
		return map[string]any{"jobs": jobs, "count": len(jobs)}, nil
	case "users":
		users, err := b.users()
		if err != nil {
			return nil, reply(10, "CRON_USERS_READ_FAILED", "Les utilisateurs Cron n’ont pas pu être lus.")
		}
		return map[string]any{"users": users, "count": len(users)}, nil
	case "run-result":
		return b.runResult(args[0])
	case "backup-create":
		return b.createBackupTask(args)
	case "backup-delete":
		return b.deleteBackupTask(args[0])
	case "backup-run":
		return b.runBackupTask(ctx, args[0])
	default:
		return b.manage(ctx, command, args)
	}
}

func (b *Backend) property(ctx context.Context, property, fallback string) string {
	value, _ := b.runner.Run(ctx, b.paths.Systemctl, "show", "--property="+property, "--value", "--", b.unit)
	if value == "" || value == "[not set]" {
		return fallback
	}
	return value
}

func (b *Backend) status(ctx context.Context) map[string]any {
	load := b.property(ctx, "LoadState", "not-found")
	exists := load == "loaded" || load == "masked"
	data := map[string]any{"service": b.service, "unit": b.unit, "exists": exists, "active": false, "enabled": false, "load_state": load, "active_state": "inactive", "state": "unknown", "main_pid": int64(0), "memory_bytes": int64(0), "tasks": int64(0)}
	if !exists {
		return data
	}
	_, active := b.runner.Run(ctx, b.paths.Systemctl, "is-active", "--quiet", "--", b.unit)
	_, enabled := b.runner.Run(ctx, b.paths.Systemctl, "is-enabled", "--quiet", "--", b.unit)
	data["active"], data["enabled"] = active == 0, enabled == 0
	data["active_state"] = b.property(ctx, "ActiveState", "inactive")
	data["state"] = b.property(ctx, "SubState", "unknown")
	for property, key := range map[string]string{"MainPID": "main_pid", "MemoryCurrent": "memory_bytes", "TasksCurrent": "tasks"} {
		value, err := strconv.ParseInt(b.property(ctx, property, "0"), 10, 64)
		if err == nil && value >= 0 {
			data[key] = value
		}
	}
	return data
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0
}
func secureExecutable(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 || info.Mode().Perm()&0022 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0
}
func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
func reply(exitCode int, code, message string) *protocol.Reply {
	result := fail(exitCode, code, message)
	return &result
}
func fail(exitCode int, code, message string) protocol.Reply {
	return protocol.Reply{ExitCode: exitCode, Response: api.Failure(code, message)}
}
