package apache

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

const (
	apachectlCommand  = "/usr/sbin/apachectl"
	apacheCommand     = "/usr/sbin/apache2"
	systemctlCommand  = "/usr/bin/systemctl"
	systemdRunCommand = "/usr/bin/systemd-run"
	a2ensiteCommand   = "/usr/sbin/a2ensite"
	a2dissiteCommand  = "/usr/sbin/a2dissite"
)

type commandResult struct {
	Output string
	OK     bool
}

type commandRunner interface {
	Run(context.Context, string, ...string) (commandResult, *Error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, arguments ...string) (commandResult, *Error) {
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, name, arguments...).CombinedOutput()
	text := strings.TrimSpace(string(output))
	if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
		return commandResult{}, domainError(124, "APACHE_COMMAND_TIMEOUT", "La commande Apache a dépassé le délai autorisé.", nil)
	}
	if err == nil {
		return commandResult{Output: text, OK: true}, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return commandResult{Output: text, OK: false}, nil
	}
	return commandResult{}, domainError(10, "APACHE_COMMAND_FAILED", "La commande Apache n’a pas pu être exécutée.", map[string]any{"reason": err.Error()})
}
