package cron

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"aegisadmin/backend/internal/protocol"
)

const maximumResultBytes = 256 * 1024

type ExecutionResult struct {
	ExecutionID string `json:"execution_id"`
	Status      string `json:"status"`
	ExitCode    *int   `json:"exit_code"`
	TimedOut    bool   `json:"timed_out"`
	Truncated   bool   `json:"truncated"`
	DurationMS  int64  `json:"duration_ms"`
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
}

func (b *Backend) runResult(id string) (map[string]any, *protocol.Reply) {
	if !executionID.MatchString(id) {
		return nil, reply(2, "INVALID_CRON_EXECUTION_ID", "L’identifiant d’exécution Cron est invalide.")
	}
	directory, err := os.Lstat(b.paths.ResultDirectory)
	if os.IsNotExist(err) {
		return nil, reply(5, "CRON_EXECUTION_NOT_FOUND", "L’exécution Cron demandée est introuvable.")
	}
	if err != nil || !secureRootMode(directory, true) {
		return nil, reply(10, "CRON_EXECUTION_READ_FAILED", "Le résultat de l’exécution Cron n’a pas pu être lu.")
	}
	path := filepath.Join(b.paths.ResultDirectory, id+".json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, reply(5, "CRON_EXECUTION_NOT_FOUND", "L’exécution Cron demandée est introuvable.")
	}
	if err != nil || !secureRootMode(info, false) || info.Size() > maximumResultBytes {
		return nil, reply(10, "CRON_EXECUTION_READ_FAILED", "Le résultat de l’exécution Cron n’a pas pu être lu.")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, reply(10, "CRON_EXECUTION_READ_FAILED", "Le résultat de l’exécution Cron n’a pas pu être lu.")
	}
	var result ExecutionResult
	var fields map[string]json.RawMessage
	if json.Unmarshal(content, &fields) != nil {
		return nil, reply(10, "CRON_EXECUTION_READ_FAILED", "Le résultat de l’exécution Cron n’a pas pu être lu.")
	}
	for _, required := range []string{"execution_id", "status", "exit_code", "timed_out", "truncated", "duration_ms", "stdout", "stderr"} {
		if _, exists := fields[required]; !exists {
			return nil, reply(10, "CRON_EXECUTION_READ_FAILED", "Le résultat de l’exécution Cron n’a pas pu être lu.")
		}
	}
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	if decoder.Decode(&result) != nil || result.ExecutionID != id || !validResult(result) {
		return nil, reply(10, "CRON_EXECUTION_READ_FAILED", "Le résultat de l’exécution Cron n’a pas pu être lu.")
	}
	if result.Status == "running" && b.now().Sub(info.ModTime()) > 75*time.Second {
		result = ExecutionResult{id, "failed", nil, true, false, 75000, "", "Le résultat de l’exécution Cron n’est pas disponible."}
	}
	encoded, _ := json.Marshal(result)
	data := map[string]any{}
	_ = json.Unmarshal(encoded, &data)
	return data, nil
}

func validResult(result ExecutionResult) bool {
	if result.Status != "running" && result.Status != "finished" && result.Status != "failed" {
		return false
	}
	return result.DurationMS >= 0 && len([]byte(result.Stdout))+len([]byte(result.Stderr)) <= maximumResultBytes
}
func secureRootMode(info os.FileInfo, directory bool) bool {
	if directory && !info.IsDir() || !directory && !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0
}
