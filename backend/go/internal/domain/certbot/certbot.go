package certbot

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

type Runner interface {
	Run(context.Context, string, ...string) (string, int)
}
type Paths struct {
	Certbot, DpkgQuery, RPM, Systemctl, SystemdRun, Runner             string
	RenewalDirectory, LiveDirectory, ArchiveDirectory, ResultDirectory string
}
type Backend struct {
	runner    Runner
	paths     Paths
	now       func() time.Time
	infoMu    sync.Mutex
	infoCache map[string]any
}
type Handler struct{ backend *Backend }
type execRunner struct{}

var fixedArguments = map[string]int{"info": 0, "status": 0, "certificates": 0, "delete": 1, "reinstall": 1, "renew-replace": 1, "renew-test": 0, "renew": 0, "action-result": 1}
var executionPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var certificateActionName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)
var runnerPath = "/usr/local/lib/aegisadmin-system/libexec/certbot-runner.py"

func New(backend *Backend) *Handler { return &Handler{backend} }
func NewLinuxBackend() *Backend {
	certbot := "/usr/bin/certbot"
	if !executable(certbot) && executable("/usr/local/bin/certbot") {
		certbot = "/usr/local/bin/certbot"
	}
	return &Backend{runner: execRunner{}, now: time.Now, paths: Paths{
		Certbot: certbot, DpkgQuery: "/usr/bin/dpkg-query", RPM: "/usr/bin/rpm", Systemctl: "/usr/bin/systemctl", SystemdRun: "/usr/bin/systemd-run",
		Runner: runnerPath, RenewalDirectory: "/etc/letsencrypt/renewal",
		LiveDirectory: "/etc/letsencrypt/live", ArchiveDirectory: "/etc/letsencrypt/archive", ResultDirectory: "/run/aegisadmin-system/certbot",
	}}
}
func (execRunner) Run(ctx context.Context, name string, args ...string) (string, int) {
	c, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, name, args...)
	cmd.Env = []string{"HOME=/root", "PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(strings.ToValidUTF8(string(out), "�"))
	if err == nil {
		return text, 0
	}
	if errors.Is(c.Err(), context.DeadlineExceeded) {
		return text, -2
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return text, exit.ExitCode()
	}
	return text, -1
}

func (h *Handler) Handle(ctx context.Context, command string, args []string) protocol.Reply {
	if command == "" {
		return fail(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine certbot.")
	}
	if command == "issue" {
		if len(args) < 3 || len(args) > 22 {
			return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
		}
	} else {
		expected, ok := fixedArguments[command]
		if !ok {
			return fail(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine certbot.")
		}
		if len(args) != expected {
			return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
		}
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
	for _, item := range []struct{ path, label string }{{b.paths.Certbot, "certbot"}, {b.paths.Systemctl, "systemctl"}} {
		if !executable(item.path) {
			return reply(3, "DEPENDENCY_NOT_FOUND", "La dépendance "+item.label+" nécessaire au domaine certbot est introuvable.")
		}
	}
	if command != "info" && command != "status" && command != "certificates" {
		if !executable(b.paths.SystemdRun) {
			return reply(3, "DEPENDENCY_NOT_FOUND", "La dépendance systemd-run nécessaire au domaine certbot est introuvable.")
		}
		if !executable(b.paths.Runner) {
			return reply(3, "DEPENDENCY_NOT_FOUND", "La dépendance certbot-runner.py nécessaire au domaine certbot est introuvable.")
		}
	}
	return nil
}
func (b *Backend) execute(ctx context.Context, command string, args []string) (map[string]any, *protocol.Reply) {
	switch command {
	case "info":
		return b.info(ctx)
	case "status":
		return b.status(ctx)
	case "certificates":
		return b.certificates(ctx)
	case "action-result":
		return b.actionResult(args[0])
	}
	parameters := map[string]any(nil)
	if command == "issue" {
		var failure *protocol.Reply
		parameters, failure = issueParameters(args)
		if failure != nil {
			return nil, failure
		}
	}
	if command == "delete" || command == "reinstall" || command == "renew-replace" {
		if !certificateActionName.MatchString(args[0]) {
			return nil, reply(2, "INVALID_CERTBOT_CERTIFICATE_NAME", "Le nom du certificat Certbot est invalide.")
		}
		parameters = map[string]any{"certificate_name": args[0]}
	}
	return b.startAction(ctx, command, parameters)
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
func reply(exit int, code, message string) *protocol.Reply {
	value := fail(exit, code, message)
	return &value
}
func fail(exit int, code, message string) protocol.Reply {
	return protocol.Reply{ExitCode: exit, Response: api.Failure(code, message)}
}
