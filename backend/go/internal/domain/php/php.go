package php

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

const (
	systemctlCommand  = "/usr/bin/systemctl"
	systemdRunCommand = "/usr/bin/systemd-run"
)

var argumentCounts = map[string]int{"info": 0, "fpm": 0, "configuration": 1, "extensions": 1, "restart": 1}
var fpmUnitPattern = regexp.MustCompile(`^php([0-9]+)\.([0-9]+)-fpm\.service$`)
var genericFPMUnitPattern = regexp.MustCompile(`^php-fpm\.service$`)
var fpmRuntimePattern = regexp.MustCompile(`^fpm-([0-9]+)\.([0-9]+)$`)

var importantDirectives = []string{
	"memory_limit", "max_execution_time", "max_input_time", "post_max_size",
	"upload_max_filesize", "max_file_uploads", "date.timezone", "display_errors",
	"log_errors", "error_log", "opcache.enable", "opcache.memory_consumption",
}

const versionSource = `echo PHP_MAJOR_VERSION, ".", PHP_MINOR_VERSION;`

type Error struct {
	ExitCode int
	Code     string
	Message  string
	Details  map[string]any
}

type commandResult struct {
	Output string
	OK     bool
}

type commandRunner interface {
	Run(context.Context, map[string]string, string, ...string) (commandResult, *Error)
}

type execRunner struct{}

type Backend struct {
	runner commandRunner
	binDir string
}

type Handler struct{ backend *Backend }

type Instance struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	Service      string `json:"service"`
	Unit         string `json:"unit"`
	Exists       bool   `json:"exists"`
	Active       bool   `json:"active"`
	Enabled      bool   `json:"enabled"`
	State        string `json:"state"`
	binary       string
	debianLayout bool
}

type runtime struct {
	ID, Type, Binary, Version string
	Instance                  *Instance
}

func New(backend *Backend) *Handler { return &Handler{backend: backend} }
func NewLinuxBackend() *Backend     { return &Backend{runner: execRunner{}, binDir: "/usr/bin"} }

func (h *Handler) Handle(ctx context.Context, command string, arguments []string) protocol.Reply {
	if command == "" {
		return failure(domainError(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine php.", nil))
	}
	expected, known := argumentCounts[command]
	if !known {
		return failure(domainError(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine php.", nil))
	}
	if len(arguments) != expected {
		return failure(domainError(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.", nil))
	}
	argument := ""
	if expected == 1 {
		argument = arguments[0]
	}
	data, err := h.backend.execute(ctx, command, argument)
	if err != nil {
		return failure(err)
	}
	return protocol.Reply{Response: api.Success(data)}
}

func (b *Backend) execute(ctx context.Context, command, argument string) (map[string]any, *Error) {
	switch command {
	case "info":
		return b.info(ctx)
	case "fpm":
		instances, err := b.instances(ctx)
		return map[string]any{"instances": instances}, err
	case "configuration":
		return b.configuration(ctx, argument)
	case "extensions":
		return b.extensions(ctx, argument)
	case "restart":
		return b.restart(ctx, argument)
	}
	return nil, domainError(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine php.", nil)
}

func (execRunner) Run(ctx context.Context, environment map[string]string, name string, arguments ...string) (commandResult, *Error) {
	commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(commandCtx, name, arguments...)
	env := map[string]string{"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C", "LC_ALL": "C"}
	for key, value := range environment {
		env[key] = value
	}
	command.Env = make([]string, 0, len(env))
	for key, value := range env {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	text := strings.TrimSpace(strings.ToValidUTF8(string(output), "�"))
	if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
		return commandResult{}, domainError(124, "PHP_COMMAND_TIMEOUT", "La commande PHP a dépassé le délai autorisé.", nil)
	}
	if err == nil {
		return commandResult{Output: text, OK: true}, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return commandResult{Output: text}, nil
	}
	return commandResult{}, domainError(10, "PHP_COMMAND_FAILED", "La commande PHP n’a pas pu être exécutée.", map[string]any{"reason": err.Error()})
}

func (b *Backend) instances(ctx context.Context) ([]Instance, *Error) {
	result, err := b.runner.Run(ctx, nil, systemctlCommand, "list-unit-files", "--type=service", "--no-legend", "--no-pager", "php*-fpm.service")
	if err != nil {
		return nil, err
	}
	type unitVersion struct {
		unit, version string
		major, minor  int
		debianLayout  bool
	}
	units := map[string]unitVersion{}
	if result.OK {
		for _, line := range strings.Split(result.Output, "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			match := fpmUnitPattern.FindStringSubmatch(fields[0])
			if match != nil {
				major, _ := strconv.Atoi(match[1])
				minor, _ := strconv.Atoi(match[2])
				units[fields[0]] = unitVersion{unit: fields[0], version: match[1] + "." + match[2], major: major, minor: minor, debianLayout: true}
			} else if genericFPMUnitPattern.MatchString(fields[0]) {
				version, versionErr := b.cliMajorMinor(ctx)
				if versionErr != nil {
					return nil, versionErr
				}
				parts := strings.Split(version, ".")
				major, _ := strconv.Atoi(parts[0])
				minor, _ := strconv.Atoi(parts[1])
				units[fields[0]] = unitVersion{unit: fields[0], version: version, major: major, minor: minor}
			}
		}
	}
	versions := make([]unitVersion, 0, len(units))
	for _, version := range units {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].major == versions[j].major {
			if versions[i].minor == versions[j].minor {
				return versions[i].debianLayout && !versions[j].debianLayout
			}
			return versions[i].minor > versions[j].minor
		}
		return versions[i].major > versions[j].major
	})
	properties := map[string]map[string]string{}
	if len(versions) != 0 {
		arguments := []string{"show", "--property=Id", "--property=Names", "--property=LoadState", "--property=ActiveState", "--property=SubState", "--property=UnitFileState", "--"}
		for _, item := range versions {
			arguments = append(arguments, item.unit)
		}
		statusResult, statusErr := b.runner.Run(ctx, nil, systemctlCommand, arguments...)
		if statusErr != nil {
			return nil, statusErr
		}
		if statusResult.OK {
			properties = parseSystemctlProperties(statusResult.Output)
		}
	}
	instances := make([]Instance, 0, len(versions))
	seenVersions := map[string]bool{}
	for _, item := range versions {
		version := item.version
		if seenVersions[version] {
			continue
		}
		seenVersions[version] = true
		unitProperties := properties[item.unit]
		load := unitProperties["LoadState"]
		active := unitProperties["ActiveState"]
		sub := unitProperties["SubState"]
		enabled := unitProperties["UnitFileState"]
		state := sub
		if state == "" {
			state = active
		}
		if state == "" {
			state = "unknown"
		}
		binaryName := "php"
		if item.debianLayout {
			binaryName += version
		}
		instances = append(instances, Instance{
			ID: "fpm-" + version, Version: version, Service: strings.TrimSuffix(item.unit, ".service"), Unit: item.unit,
			Exists: load == "loaded" || load == "masked", Active: active == "active", Enabled: enabled == "enabled", State: state,
			binary: b.binary(binaryName), debianLayout: item.debianLayout,
		})
	}
	return instances, nil
}

func (b *Backend) cliMajorMinor(ctx context.Context) (string, *Error) {
	result, err := b.runner.Run(ctx, nil, b.binary("php"), "-r", versionSource)
	if err != nil || !result.OK || !regexp.MustCompile(`^[0-9]+\.[0-9]+$`).MatchString(result.Output) {
		return "", domainError(10, "PHP_VERSION_READ_FAILED", "La version de PHP n’a pas pu être déterminée.", nil)
	}
	return result.Output, nil
}

func parseSystemctlProperties(output string) map[string]map[string]string {
	result := map[string]map[string]string{}
	current := map[string]string{}
	flush := func() {
		if unit := current["Id"]; unit != "" {
			result[unit] = current
			for _, name := range strings.Fields(current["Names"]) {
				result[name] = current
			}
		}
		current = map[string]string{}
	}
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			flush()
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found {
			if key == "Id" && current["Id"] != "" {
				flush()
			}
			current[key] = value
		}
	}
	flush()
	return result
}

func failure(err *Error) protocol.Reply {
	response := api.Failure(err.Code, err.Message)
	if response.Error != nil && len(err.Details) != 0 {
		response.Error.Details = err.Details
	}
	return protocol.Reply{ExitCode: err.ExitCode, Response: response}
}

func domainError(exit int, code, message string, details map[string]any) *Error {
	return &Error{ExitCode: exit, Code: code, Message: message, Details: details}
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0
}

func (b *Backend) binary(name string) string { return filepath.Join(b.binDir, name) }
