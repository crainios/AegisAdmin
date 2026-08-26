package cron

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"aegisadmin/backend/internal/protocol"
)

var (
	managedID   = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[1-5][a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)
	legacyID    = regexp.MustCompile(`^legacy-[a-f0-9]{32}$`)
	executionID = regexp.MustCompile(`^[a-f0-9]{32}$`)
	macroExact  = regexp.MustCompile(`^@(reboot|yearly|annually|monthly|weekly|daily|midnight|hourly)$`)
)

type record struct {
	id, line, schedule, command string
	start, end                  int
	enabled, managed            bool
}

func (b *Backend) manage(ctx context.Context, action string, args []string) (map[string]any, *protocol.Reply) {
	owner := args[0]
	if !b.editable(owner) {
		return nil, reply(2, "INVALID_CRON_USER", "L’utilisateur Cron demandé n’est pas autorisé.")
	}
	if _, err := user.Lookup(owner); err != nil {
		return nil, reply(2, "INVALID_CRON_TASK", "Les paramètres de la tâche Cron sont invalides.")
	}
	id, schedule, command := "", "", ""
	if action == "create" {
		schedule, command = args[1], args[2]
	} else {
		id = args[1]
		if !managedID.MatchString(id) && !legacyID.MatchString(id) {
			return nil, reply(2, "INVALID_CRON_TASK_ID", "L’identifiant de la tâche Cron est invalide.")
		}
		if action == "update" {
			schedule, command = args[2], args[3]
		}
	}
	if action == "create" || action == "update" {
		if !validTaskInput(schedule, command) {
			return nil, reply(2, "INVALID_CRON_TASK", "Les paramètres de la tâche Cron sont invalides.")
		}
	}
	if err := os.MkdirAll(filepath.Dir(b.paths.LockFile), 0o755); err != nil {
		return nil, b.actionFailure(action)
	}
	lock, err := os.OpenFile(b.paths.LockFile, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, b.actionFailure(action)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return nil, b.actionFailure(action)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck

	original, status := b.runner.Run(ctx, b.paths.Crontab, "-u", owner, "-l")
	if status != 0 && status != 1 {
		return nil, b.actionFailure(action)
	}
	if status == 1 {
		original = ""
	}
	lines := splitContent(original)
	records, err := parseRecords(owner, lines)
	if err != nil {
		return nil, reply(2, "INVALID_CRON_TASK", "Les paramètres de la tâche Cron sont invalides.")
	}
	if action == "run" {
		task, failure := findRecord(records, id)
		if failure != nil {
			return nil, failure
		}
		if !task.enabled {
			return nil, reply(5, "INVALID_CRON_TASK_STATE", "L’état actuel de la tâche Cron interdit cette action.")
		}
		return b.startExecution(ctx, owner, id, task.command)
	}
	resultID, enabled := id, true
	if action == "create" {
		resultID, err = newUUID()
		if err != nil {
			return nil, b.actionFailure(action)
		}
		lines = append(lines, activeMarker(resultID), normalizeSchedule(schedule)+" "+strings.TrimSpace(command))
	} else {
		task, failure := findRecord(records, id)
		if failure != nil {
			return nil, failure
		}
		if !task.managed {
			resultID, err = newUUID()
			if err != nil {
				return nil, b.actionFailure(action)
			}
		}
		var replacement []string
		switch action {
		case "update":
			line := normalizeSchedule(schedule) + " " + strings.TrimSpace(command)
			enabled = task.enabled
			if enabled {
				replacement = []string{activeMarker(resultID), line}
			} else {
				replacement = []string{suspendedMarker(resultID, line)}
			}
		case "suspend":
			if !task.enabled {
				return nil, reply(5, "INVALID_CRON_TASK_STATE", "L’état actuel de la tâche Cron interdit cette action.")
			}
			replacement, enabled = []string{suspendedMarker(resultID, task.line)}, false
		case "resume":
			if task.enabled {
				return nil, reply(5, "INVALID_CRON_TASK_STATE", "L’état actuel de la tâche Cron interdit cette action.")
			}
			replacement = []string{activeMarker(resultID), task.line}
		case "delete":
			replacement, enabled = []string{}, false
		}
		lines = replace(lines, task.start, task.end, replacement)
	}
	if err := b.backup(owner, original); err != nil {
		return nil, b.actionFailure(action)
	}
	if err := b.install(ctx, owner, lines); err != nil {
		if err == errInvalid {
			return nil, reply(2, "INVALID_CRON_TASK", "Les paramètres de la tâche Cron sont invalides.")
		}
		return nil, b.actionFailure(action)
	}
	return map[string]any{"action": action, "user": owner, "id": resultID, "enabled": enabled, "result": "success"}, nil
}

var errInvalid = errorsText("invalid")

func validTaskInput(schedule, command string) bool {
	if schedule == "" || utf8.RuneCountInString(schedule) > 128 || strings.ContainsAny(schedule, "\x00\r\n") || strings.TrimSpace(command) == "" || utf8.RuneCountInString(command) > 8192 || strings.ContainsAny(command, "\x00\r\n") {
		return false
	}
	normalized := normalizeSchedule(schedule)
	if strings.HasPrefix(normalized, "@") {
		return macroExact.MatchString(normalized)
	}
	return len(strings.Fields(normalized)) == 5
}
func normalizeSchedule(value string) string { return strings.Join(strings.Fields(value), " ") }
func activeMarker(id string) string         { return "# AEGISADMIN-TASK: " + id + " ACTIVE" }
func suspendedMarker(id, line string) string {
	return "# AEGISADMIN-TASK: " + id + " SUSPENDED " + base64.RawURLEncoding.EncodeToString([]byte(line))
}

func splitContent(content string) []string {
	content = strings.TrimSuffix(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if content == "" {
		return []string{}
	}
	return strings.Split(content, "\n")
}

func parseRecords(owner string, lines []string) ([]record, error) {
	records := []record{}
	legacyOccurrences := map[string]int{}
	for index := 0; index < len(lines); {
		raw, line := lines[index], strings.TrimSpace(lines[index])
		marker := managedMarker.FindStringSubmatch(line)
		if marker != nil {
			id, state, payload := marker[1], marker[2], marker[3]
			if state == "SUSPENDED" {
				decoded, err := base64.RawURLEncoding.DecodeString(payload)
				if err != nil || len(decoded) == 0 || strings.ContainsAny(string(decoded), "\r\n") {
					return nil, errInvalid
				}
				schedule, _, command, err := parseJobLine(string(decoded), owner)
				if err != nil {
					return nil, err
				}
				records = append(records, record{id, string(decoded), schedule, command, index, index, false, true})
				index++
				continue
			}
			if payload != "" || index+1 >= len(lines) {
				return nil, errInvalid
			}
			taskLine := lines[index+1]
			if strings.TrimSpace(taskLine) == "" || strings.HasPrefix(strings.TrimSpace(taskLine), "#") || environment.MatchString(strings.TrimSpace(taskLine)) {
				return nil, errInvalid
			}
			schedule, _, command, err := parseJobLine(taskLine, owner)
			if err != nil {
				return nil, err
			}
			records = append(records, record{id, taskLine, schedule, command, index, index + 1, true, true})
			index += 2
			continue
		}
		if strings.HasPrefix(line, "# AEGISADMIN-TASK:") {
			return nil, errInvalid
		}
		if line == "" || strings.HasPrefix(line, "#") || environment.MatchString(line) {
			index++
			continue
		}
		schedule, _, command, err := parseJobLine(raw, owner)
		if err != nil {
			return nil, err
		}
		key := normalizeSchedule(schedule) + "\x00" + strings.TrimSpace(command)
		legacyOccurrences[key]++
		id := legacyTaskIdentifier(owner, schedule, command, legacyOccurrences[key])
		records = append(records, record{id, raw, schedule, command, index, index, true, false})
		index++
	}
	return records, nil
}

func findRecord(records []record, id string) (record, *protocol.Reply) {
	matches := []record{}
	for _, item := range records {
		if item.id == id {
			matches = append(matches, item)
		}
	}
	if len(matches) != 1 {
		return record{}, reply(5, "CRON_TASK_NOT_FOUND", "La tâche Cron demandée est introuvable.")
	}
	return matches[0], nil
}
func replace(lines []string, start, end int, replacement []string) []string {
	result := append([]string{}, lines[:start]...)
	result = append(result, replacement...)
	return append(result, lines[end+1:]...)
}

func newUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:]), nil
}

func secureDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errorsText("insecure directory")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || stat.Uid != 0 {
		return errorsText("insecure directory")
	}
	return nil
}

func (b *Backend) backup(owner, content string) error {
	if err := secureDirectory(b.paths.BackupDirectory); err != nil {
		return err
	}
	stamp := b.now().UTC().Format("20060102T150405.000000Z")
	path := filepath.Join(b.paths.BackupDirectory, stamp+"-"+owner+".crontab")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		return err
	}
	return file.Sync()
}

func (b *Backend) install(ctx context.Context, owner string, lines []string) error {
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	file, err := os.CreateTemp(b.paths.TempDirectory, "aegisadmin-cron-")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.WriteString(content); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if output, status := b.runner.Run(ctx, b.paths.Crontab, "-u", owner, "-n", path); status != 0 && !unsupportedCronValidation(output) {
		return errInvalid
	}
	if _, status := b.runner.Run(ctx, b.paths.Crontab, "-u", owner, path); status != 0 {
		return errorsText("installation failed")
	}
	return nil
}

func unsupportedCronValidation(output string) bool {
	message := strings.ToLower(output)
	return strings.Contains(message, "invalid option") ||
		strings.Contains(message, "illegal option") ||
		strings.Contains(message, "unknown option") ||
		(strings.Contains(message, "usage:") && strings.Contains(message, "crontab"))
}

func (b *Backend) startExecution(ctx context.Context, owner, taskID, command string) (map[string]any, *protocol.Reply) {
	if !secureExecutable(b.paths.Runner) {
		return nil, b.actionFailure("run")
	}
	if err := secureDirectory(b.paths.ResultDirectory); err != nil {
		return nil, b.actionFailure("run")
	}
	entries, _ := os.ReadDir(b.paths.ResultDirectory)
	threshold := b.now().Add(-24 * time.Hour)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			if info, err := entry.Info(); err == nil && info.Mode().IsRegular() && info.ModTime().Before(threshold) {
				_ = os.Remove(filepath.Join(b.paths.ResultDirectory, entry.Name()))
			}
		}
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return nil, b.actionFailure("run")
	}
	execution := hex.EncodeToString(random)
	pending := map[string]any{"execution_id": execution, "status": "running", "exit_code": nil, "timed_out": false, "truncated": false, "duration_ms": 0, "stdout": "", "stderr": ""}
	encoded, _ := json.Marshal(pending)
	resultPath := filepath.Join(b.paths.ResultDirectory, execution+".json")
	temporary, err := os.OpenFile(filepath.Join(b.paths.ResultDirectory, "."+execution+".tmp"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, b.actionFailure("run")
	}
	temporaryPath := temporary.Name()
	if _, err = temporary.Write(append(encoded, '\n')); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temporaryPath, resultPath)
	}
	if err != nil {
		_ = os.Remove(temporaryPath)
		return nil, b.actionFailure("run")
	}
	unitName := "aegisadmin-cron-run-" + execution
	_, status := b.runner.Run(ctx, b.paths.SystemdRun, "--quiet", "--collect", "--unit="+unitName, "--property=Type=exec", "--property=RuntimeMaxSec=70s", b.paths.Runner, execution, owner, command)
	if status != 0 {
		_ = os.Remove(resultPath)
		return nil, b.actionFailure("run")
	}
	return map[string]any{"action": "run", "user": owner, "id": taskID, "execution_id": execution, "result": "scheduled"}, nil
}

func (b *Backend) actionFailure(action string) *protocol.Reply {
	codes := map[string][2]string{
		"create": {"CRON_TASK_CREATE_FAILED", "La création de la tâche Cron a échoué."}, "update": {"CRON_TASK_UPDATE_FAILED", "La modification de la tâche Cron a échoué."},
		"suspend": {"CRON_TASK_SUSPEND_FAILED", "La suspension de la tâche Cron a échoué."}, "resume": {"CRON_TASK_RESUME_FAILED", "La réactivation de la tâche Cron a échoué."},
		"delete": {"CRON_TASK_DELETE_FAILED", "La suppression de la tâche Cron a échoué."}, "run": {"CRON_TASK_RUN_FAILED", "Le lancement de la tâche Cron a échoué."},
	}
	value := codes[action]
	return reply(10, value[0], value[1])
}
