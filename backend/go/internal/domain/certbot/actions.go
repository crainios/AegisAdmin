package certbot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"aegisadmin/backend/internal/protocol"
)

var domainPattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var emailPattern = regexp.MustCompile(`^[A-Za-z0-9.!#$%&'*+/=?^_{}|~-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+$`)
var validActions = map[string]bool{"renew-test": true, "renew": true, "issue": true, "delete": true, "reinstall": true, "renew-replace": true}

func issueParameters(args []string) (map[string]any, *protocol.Reply) {
	email, redirect := args[0], args[1]
	if utf8.RuneCountInString(email) < 3 || utf8.RuneCountInString(email) > 254 || !emailPattern.MatchString(email) || (redirect != "redirect" && redirect != "no-redirect") {
		return nil, reply(2, "INVALID_CERTBOT_ISSUE_PARAMETERS", "Les paramètres d’émission du certificat sont invalides.")
	}
	domains := []string{}
	seen := map[string]bool{}
	for _, domain := range args[2:] {
		normalized := strings.ToLower(strings.TrimSpace(domain))
		if domain != normalized || utf8.RuneCountInString(normalized) < 1 || utf8.RuneCountInString(normalized) > 253 || !domainPattern.MatchString(normalized) || strings.HasSuffix(normalized, ".onion") || seen[normalized] {
			return nil, reply(2, "INVALID_CERTBOT_ISSUE_PARAMETERS", "Les paramètres d’émission du certificat sont invalides.")
		}
		seen[normalized] = true
		domains = append(domains, normalized)
	}
	return map[string]any{"domains": domains, "email": email, "redirect": redirect == "redirect"}, nil
}

func (b *Backend) startAction(ctx context.Context, action string, parameters map[string]any) (map[string]any, *protocol.Reply) {
	if !validActions[action] {
		return nil, reply(10, "INVALID_CERTBOT_ACTION", "L’action Certbot demandée est invalide.")
	}
	if err := secureDirectory(b.paths.ResultDirectory); err != nil {
		return nil, reply(10, "CERTBOT_ACTION_PREPARE_FAILED", "L’exécution Certbot n’a pas pu être préparée.")
	}
	var execution, path string
	for attempt := 0; attempt < 10; attempt++ {
		random := make([]byte, 16)
		if _, err := rand.Read(random); err != nil {
			continue
		}
		execution = hex.EncodeToString(random)
		path = filepath.Join(b.paths.ResultDirectory, execution+".json")
		payload := map[string]any{"execution_id": execution, "action": action, "status": "running", "exit_code": nil, "timed_out": false, "truncated": false, "duration_ms": 0, "stdout": "", "stderr": "", "created_at": b.now().Unix()}
		if parameters != nil {
			payload["parameters"] = parameters
		}
		encoded, _ := json.Marshal(payload)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return nil, reply(10, "CERTBOT_ACTION_PREPARE_FAILED", "L’exécution Certbot n’a pas pu être préparée.")
		}
		ownershipErr := file.Chown(0, 0)
		_, writeErr := file.Write(append(encoded, '\n'))
		closeErr := file.Close()
		if ownershipErr != nil || writeErr != nil || closeErr != nil {
			_ = os.Remove(path)
			return nil, reply(10, "CERTBOT_ACTION_PREPARE_FAILED", "L’exécution Certbot n’a pas pu être préparée.")
		}
		break
	}
	if execution == "" {
		return nil, reply(10, "CERTBOT_ACTION_PREPARE_FAILED", "L’exécution Certbot n’a pas pu être préparée.")
	}
	unit := "aegisadmin-certbot-" + action + "-" + execution
	_, status := b.runner.Run(ctx, b.paths.SystemdRun, "--quiet", "--collect", "--unit="+unit, "--property=Type=exec", "--property=User=root", "--property=Group=root", "--property=PrivateTmp=true", "--property=NoNewPrivileges=true", "--property=RuntimeMaxSec=930", "--", b.paths.Runner, action, execution)
	if status != 0 {
		_ = os.Remove(path)
		return nil, reply(10, "CERTBOT_ACTION_SCHEDULE_FAILED", "L’exécution Certbot n’a pas pu être programmée.")
	}
	return map[string]any{"action": action, "execution_id": execution, "result": "scheduled"}, nil
}

const maxResultBytes = 512 * 1024
const maxOutputBytes = 128 * 1024

type ActionResult struct {
	ExecutionID string `json:"execution_id"`
	Action      string `json:"action"`
	Status      string `json:"status"`
	ExitCode    *int   `json:"exit_code"`
	TimedOut    bool   `json:"timed_out"`
	Truncated   bool   `json:"truncated"`
	DurationMS  int64  `json:"duration_ms"`
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
}

func (b *Backend) actionResult(id string) (map[string]any, *protocol.Reply) {
	if !executionPattern.MatchString(id) {
		return nil, reply(2, "INVALID_CERTBOT_EXECUTION_ID", "L’identifiant d’exécution Certbot est invalide.")
	}
	directory, err := os.Lstat(b.paths.ResultDirectory)
	if os.IsNotExist(err) {
		return nil, reply(5, "CERTBOT_EXECUTION_NOT_FOUND", "L’exécution Certbot demandée est introuvable.")
	}
	if err != nil || !secureRootMode(directory, true) {
		return nil, b.resultFailure()
	}
	path := filepath.Join(b.paths.ResultDirectory, id+".json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, reply(5, "CERTBOT_EXECUTION_NOT_FOUND", "L’exécution Certbot demandée est introuvable.")
	}
	if err != nil || !secureRootMode(info, false) || info.Size() > maxResultBytes {
		return nil, b.resultFailure()
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, b.resultFailure()
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(content, &fields) != nil {
		return nil, b.resultFailure()
	}
	for _, key := range []string{"execution_id", "action", "status", "exit_code", "timed_out", "truncated", "duration_ms", "stdout", "stderr"} {
		if _, ok := fields[key]; !ok {
			return nil, b.resultFailure()
		}
	}
	var result ActionResult
	if json.Unmarshal(content, &result) != nil || result.ExecutionID != id || !validActions[result.Action] || (result.Status != "running" && result.Status != "finished" && result.Status != "failed") || result.DurationMS < 0 || len([]byte(result.Stdout))+len([]byte(result.Stderr)) > maxOutputBytes {
		return nil, b.resultFailure()
	}
	if result.Status == "running" && b.now().Sub(info.ModTime()) > 930*time.Second {
		result.Status = "failed"
		result.ExitCode = nil
		result.TimedOut = true
		result.Truncated = false
		result.DurationMS = 930000
		result.Stdout = ""
		result.Stderr = "Le résultat de l’exécution Certbot n’est pas disponible."
	}
	encoded, _ := json.Marshal(result)
	data := map[string]any{}
	_ = json.Unmarshal(encoded, &data)
	return data, nil
}
func secureDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := os.Chown(path, 0, 0); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !secureRootMode(info, true) {
		return errorsText("insecure directory")
	}
	return nil
}
func secureRootMode(info os.FileInfo, directory bool) bool {
	if directory && !info.IsDir() || !directory && !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0 && stat.Gid == 0
}
func (b *Backend) resultFailure() *protocol.Reply {
	return reply(10, "CERTBOT_EXECUTION_READ_FAILED", "Le résultat de l’exécution Certbot n’a pas pu être lu.")
}
