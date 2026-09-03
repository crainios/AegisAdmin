package cron

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"aegisadmin/backend/internal/backup"
	"aegisadmin/backend/internal/protocol"
)

var backupLabel = regexp.MustCompile(`^[[:alnum:]À-ÿ][[:alnum:]À-ÿ ._-]{0,63}$`)

func (b *Backend) createBackupTask(args []string) (map[string]any, *protocol.Reply) {
	name, kind := strings.TrimSpace(args[0]), args[1]
	source, destination, schedule := strings.TrimSpace(args[2]), strings.TrimSpace(args[3]), normalizeSchedule(args[4])
	port, portErr := strconv.Atoi(args[7])
	retention, retentionErr := strconv.Atoi(args[10])
	removeLocal, boolErr := strconv.ParseBool(args[11])
	if !backupLabel.MatchString(name) || (kind != "mysql" && kind != "apache" && kind != "sites") || !validTaskInput(schedule, "true") || portErr != nil || retentionErr != nil || boolErr != nil {
		return nil, reply(2, "INVALID_BACKUP_TASK", "Les paramètres de la sauvegarde sont invalides.")
	}
	id, err := newUUID()
	if err != nil {
		return nil, reply(10, "BACKUP_TASK_CREATE_FAILED", "La création de la sauvegarde a échoué.")
	}
	task := backup.Task{ID: id, Name: name, Kind: kind, Source: source, Destination: destination, RemoteHost: strings.TrimSpace(args[5]), RemoteUser: strings.TrimSpace(args[6]), RemotePort: port, RemotePath: strings.TrimSpace(args[8]), SSHKey: strings.TrimSpace(args[9]), RetentionDays: retention, RemoveLocal: removeLocal}
	if err = backup.Validate(task); err != nil {
		return nil, reply(2, "INVALID_BACKUP_TASK", err.Error())
	}
	if !secureExecutable(b.paths.BackupExecutable) {
		return nil, reply(3, "DEPENDENCY_NOT_FOUND", "L’exécuteur de sauvegarde sécurisé est introuvable.")
	}
	encoded, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return nil, reply(10, "BACKUP_TASK_CREATE_FAILED", "La création de la sauvegarde a échoué.")
	}
	if err = secureDirectory(b.paths.BackupTaskDirectory); err != nil {
		return nil, reply(10, "BACKUP_TASK_CREATE_FAILED", "Le stockage sécurisé des tâches est indisponible.")
	}
	configPath := filepath.Join(b.paths.BackupTaskDirectory, id+".json")
	if err = os.WriteFile(configPath, append(encoded, '\n'), 0o600); err != nil {
		return nil, reply(10, "BACKUP_TASK_CREATE_FAILED", "La configuration de la sauvegarde n’a pas pu être écrite.")
	}
	cronPath := filepath.Join(b.paths.BackupCronDirectory, "aegisadmin-backup-"+id)
	content := "# Sauvegarde gérée par AegisAdmin : " + name + "\n" + schedule + " root " + b.paths.BackupExecutable + " " + id + "\n"
	if err = os.WriteFile(cronPath, []byte(content), 0o600); err != nil {
		_ = os.Remove(configPath)
		return nil, reply(10, "BACKUP_TASK_CREATE_FAILED", "La planification de la sauvegarde a échoué.")
	}
	return map[string]any{"id": id, "name": name, "kind": kind, "result": "success"}, nil
}

func (b *Backend) deleteBackupTask(id string) (map[string]any, *protocol.Reply) {
	if !managedID.MatchString(id) {
		return nil, reply(2, "INVALID_BACKUP_TASK", "L’identifiant de sauvegarde est invalide.")
	}
	config := filepath.Join(b.paths.BackupTaskDirectory, id+".json")
	cron := filepath.Join(b.paths.BackupCronDirectory, "aegisadmin-backup-"+id)
	if _, err := os.Lstat(config); err != nil {
		return nil, reply(5, "BACKUP_TASK_NOT_FOUND", "La tâche de sauvegarde est introuvable.")
	}
	if err := os.Remove(cron); err != nil && !os.IsNotExist(err) {
		return nil, reply(10, "BACKUP_TASK_DELETE_FAILED", "La planification n’a pas pu être supprimée.")
	}
	if err := os.Remove(config); err != nil {
		return nil, reply(10, "BACKUP_TASK_DELETE_FAILED", "La configuration n’a pas pu être supprimée.")
	}
	return map[string]any{"id": id, "result": "success"}, nil
}

func (b *Backend) runBackupTask(ctx context.Context, id string) (map[string]any, *protocol.Reply) {
	if !managedID.MatchString(id) {
		return nil, reply(2, "INVALID_BACKUP_TASK", "L’identifiant de sauvegarde est invalide.")
	}
	if _, err := os.Lstat(filepath.Join(b.paths.BackupTaskDirectory, id+".json")); err != nil {
		return nil, reply(5, "BACKUP_TASK_NOT_FOUND", "La tâche de sauvegarde est introuvable.")
	}
	return b.startExecution(ctx, "root", id, b.paths.BackupExecutable+" "+id)
}
