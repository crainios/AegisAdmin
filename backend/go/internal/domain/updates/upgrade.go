package updates

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

const (
	upgradeTimeout  = 2 * time.Hour
	maximumLogLines = 400
	maximumLineSize = 2048
)

type upgradeJob struct {
	ID            string   `json:"job_id"`
	Status        string   `json:"status"`
	Backend       string   `json:"backend"`
	Lines         []string `json:"lines"`
	ExitCode      *int     `json:"exit_code"`
	StartedAt     string   `json:"started_at"`
	FinishedAt    *string  `json:"finished_at"`
	VersionBefore string   `json:"version_before,omitempty"`
	VersionAfter  string   `json:"version_after,omitempty"`
}

type upgradeStore struct{ root string }
type updaterCommands struct{ aptGet, dnf string }

func (b *Backend) startUpgrade() protocol.Reply {
	b.upgradeMu.Lock()
	defer b.upgradeMu.Unlock()
	store := upgradeStore{root: b.updateState}
	if err := store.validate(); err != nil {
		return fail(10, "UPDATE_STATE_INVALID", "Le répertoire de suivi des mises à jour n’est pas sécurisé.")
	}
	if current, err := store.current(); err == nil {
		if job, readErr := store.read(current); readErr == nil && (job.Status == "pending" || job.Status == "running") {
			unit := "aegisadmin-updater@" + job.ID + ".service"
			active := -1
			if b.runner != nil && executable(b.systemctl) {
				_, active = b.runner.Run(context.Background(), b.systemctl, "is-active", "--quiet", unit)
			}
			started, _ := time.Parse(time.RFC3339, job.StartedAt)
			if active == 0 || time.Since(started) < 30*time.Second {
				return protocol.Reply{Response: api.Success(map[string]any{"job_id": job.ID, "already_running": true})}
			}
			exitCode := -1
			job.Status, job.ExitCode = "failed", &exitCode
			finished := time.Now().UTC().Format(time.RFC3339)
			job.FinishedAt = &finished
			appendUpgradeLine(&job, "Le service de mise à jour n’est plus actif ; le travail a été clôturé.")
			_ = store.write(job)
		}
	}
	manager := ""
	if executable(b.apt) && executable(b.aptGet) {
		manager = "apt"
	} else if executable(b.dnf) {
		manager = "dnf"
	} else {
		return fail(3, "DEPENDENCY_NOT_FOUND", "Aucun gestionnaire de mises à jour APT ou DNF pris en charge n’est installé.")
	}
	if !executable(b.systemctl) {
		return fail(3, "DEPENDENCY_NOT_FOUND", "systemd est nécessaire pour isoler l’installation des mises à jour.")
	}
	id, err := newJobID()
	if err != nil {
		return fail(10, "JOB_ID_GENERATION_FAILED", "Le suivi de la mise à jour n’a pas pu être initialisé.")
	}
	job := upgradeJob{ID: id, Status: "pending", Backend: manager, Lines: []string{"Préparation du service indépendant de mise à jour…"}, StartedAt: time.Now().UTC().Format(time.RFC3339), VersionBefore: installedVersion()}
	if err := store.create(job); err != nil {
		return fail(10, "UPDATE_JOB_CREATE_FAILED", "Le travail de mise à jour n’a pas pu être enregistré.")
	}
	unit := "aegisadmin-updater@" + id + ".service"
	_, status := b.runner.Run(context.Background(), b.systemctl, "start", "--no-block", unit)
	if status != 0 {
		exitCode := -1
		job.Status, job.ExitCode = "failed", &exitCode
		finished := time.Now().UTC().Format(time.RFC3339)
		job.FinishedAt = &finished
		job.Lines = append(job.Lines, "Le service indépendant de mise à jour n’a pas pu démarrer.")
		_ = store.write(job)
		return fail(10, "UPDATE_SERVICE_START_FAILED", "Le service indépendant de mise à jour n’a pas pu démarrer.")
	}
	return protocol.Reply{Response: api.Success(map[string]any{"job_id": id, "already_running": false})}
}

func (b *Backend) upgradeStatus(id string) protocol.Reply {
	if !validJobID(id) {
		return fail(2, "INVALID_JOB_ID", "L’identifiant du travail de mise à jour est invalide.")
	}
	job, err := (upgradeStore{root: b.updateState}).read(id)
	if err != nil {
		return fail(4, "UPDATE_JOB_NOT_FOUND", "Le travail de mise à jour demandé est introuvable.")
	}
	return protocol.Reply{Response: api.Success(map[string]any{"job_id": job.ID, "status": job.Status, "backend": job.Backend, "lines": job.Lines, "exit_code": job.ExitCode, "started_at": job.StartedAt, "finished_at": job.FinishedAt, "version_before": job.VersionBefore, "version_after": job.VersionAfter})}
}

func (b *Backend) scheduleReboot(delay string) protocol.Reply {
	b.upgradeMu.Lock()
	defer b.upgradeMu.Unlock()
	allowed := map[string]bool{"0": true, "5": true, "15": true, "30": true, "60": true}
	if !allowed[delay] {
		return fail(2, "INVALID_REBOOT_DELAY", "Le délai de redémarrage demandé est invalide.")
	}
	if b.upgradeInProgress() {
		return fail(10, "UPDATE_IN_PROGRESS", "Le serveur ne peut pas redémarrer pendant une mise à jour.")
	}
	if b.runner == nil || !executable(b.systemctl) || !executable(b.systemdRun) {
		return fail(3, "DEPENDENCY_NOT_FOUND", "systemd est nécessaire pour redémarrer le serveur.")
	}
	if delay == "0" {
		// Laisse au serveur web le temps de confirmer la demande et d'afficher
		// l'écran de reconnexion avant que le système interrompe les services.
		arguments := []string{"--unit=aegisadmin-reboot", "--collect", "--on-active=5s", b.systemctl, "reboot", "--no-block"}
		if _, status := b.runner.Run(context.Background(), b.systemdRun, arguments...); status != 0 {
			return fail(10, "REBOOT_SCHEDULE_FAILED", "Le redémarrage du serveur n’a pas pu être programmé.")
		}
		return protocol.Reply{Response: api.Success(map[string]any{"scheduled": true, "delay_minutes": 0})}
	}
	arguments := []string{"--unit=aegisadmin-reboot", "--collect", "--on-active=" + delay + "m", b.systemctl, "reboot", "--no-block"}
	if _, status := b.runner.Run(context.Background(), b.systemdRun, arguments...); status != 0 {
		return fail(10, "REBOOT_SCHEDULE_FAILED", "Le redémarrage du serveur n’a pas pu être programmé.")
	}
	return protocol.Reply{Response: api.Success(map[string]any{"scheduled": true, "delay_minutes": delay})}
}

func (b *Backend) upgradeInProgress() bool {
	store := upgradeStore{root: b.updateState}
	current, err := store.current()
	if err != nil {
		return false
	}
	job, err := store.read(current)
	if err != nil || (job.Status != "pending" && job.Status != "running") {
		return false
	}
	if b.runner != nil && executable(b.systemctl) {
		if _, status := b.runner.Run(context.Background(), b.systemctl, "is-active", "--quiet", "aegisadmin-updater@"+job.ID+".service"); status == 0 {
			return true
		}
	}
	started, err := time.Parse(time.RFC3339, job.StartedAt)
	return err == nil && time.Since(started) < 30*time.Second
}

// RunUpdater executes a persisted job from the independent systemd unit.
func RunUpdater(jobID, stateDirectory string) error {
	return runUpdater(context.Background(), jobID, upgradeStore{root: stateDirectory}, updaterCommands{aptGet: aptGetCommand, dnf: dnfCommand})
}

func runUpdater(parent context.Context, jobID string, store upgradeStore, commands updaterCommands) error {
	if !validJobID(jobID) {
		return errors.New("invalid update job id")
	}
	if err := store.validate(); err != nil {
		return err
	}
	lock, err := store.lock()
	if err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); _ = lock.Close() }()
	job, err := store.read(jobID)
	if err != nil {
		return err
	}
	if job.Status != "pending" {
		return errors.New("update job is not pending")
	}
	job.Status = "running"
	appendUpgradeLine(&job, "Le service indépendant a démarré.")
	if err := store.write(job); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, upgradeTimeout)
	defer cancel()
	steps := [][]string{}
	switch job.Backend {
	case "apt":
		if !executable(commands.aptGet) {
			return finishUpgrade(store, &job, -1, "La commande apt-get est introuvable.")
		}
		steps = [][]string{
			{commands.aptGet, "update"},
			{commands.aptGet, "-y", "--with-new-pkgs", "-o", "Dpkg::Options::=--force-confold", "upgrade"},
		}
	case "dnf":
		if !executable(commands.dnf) {
			return finishUpgrade(store, &job, -1, "La commande dnf est introuvable.")
		}
		steps = [][]string{{commands.dnf, "-y", "upgrade"}}
	default:
		return finishUpgrade(store, &job, -1, "Le gestionnaire de paquets enregistré est invalide.")
	}
	for _, step := range steps {
		exitCode, runErr := runUpgradeCommand(ctx, store, &job, step[0], step[1:]...)
		if runErr != nil || exitCode != 0 {
			if ctx.Err() == context.DeadlineExceeded {
				return finishUpgrade(store, &job, -1, "La mise à jour a dépassé la durée maximale autorisée.")
			}
			return finishUpgrade(store, &job, exitCode, "La mise à jour des paquets s’est terminée avec une erreur.")
		}
	}
	job.VersionAfter = installedVersion()
	return finishUpgrade(store, &job, 0, "Mise à jour des paquets terminée avec succès.")
}

func runUpgradeCommand(ctx context.Context, store upgradeStore, job *upgradeJob, binary string, arguments ...string) (int, error) {
	appendUpgradeLine(job, fmt.Sprintf("$ %s %s", filepath.Base(binary), strings.Join(arguments, " ")))
	if err := store.write(*job); err != nil {
		return -1, err
	}
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C", "DEBIAN_FRONTEND=noninteractive"}
	pipe, err := command.StdoutPipe()
	if err == nil {
		command.Stderr = command.Stdout
		err = command.Start()
	}
	if err != nil {
		return -1, err
	}
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 4096), 256*1024)
	for scanner.Scan() {
		appendUpgradeLine(job, scanner.Text())
		if err := store.write(*job); err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			return -1, err
		}
	}
	if err := scanner.Err(); err != nil {
		appendUpgradeLine(job, "La lecture de la sortie du gestionnaire de paquets a été interrompue.")
	}
	err = command.Wait()
	if err == nil {
		return 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode(), err
	}
	return -1, err
}

func finishUpgrade(store upgradeStore, job *upgradeJob, exitCode int, message string) error {
	appendUpgradeLine(job, message)
	job.ExitCode = &exitCode
	job.Status = "completed"
	if exitCode != 0 {
		job.Status = "failed"
	}
	finished := time.Now().UTC().Format(time.RFC3339)
	job.FinishedAt = &finished
	if job.VersionAfter == "" {
		job.VersionAfter = installedVersion()
	}
	if err := store.write(*job); err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("package upgrade failed with exit code %d", exitCode)
	}
	return nil
}

func appendUpgradeLine(job *upgradeJob, line string) {
	line = strings.TrimSpace(strings.ToValidUTF8(line, "�"))
	if line == "" {
		return
	}
	if len(line) > maximumLineSize {
		line = line[:maximumLineSize] + "…"
	}
	job.Lines = append(job.Lines, line)
	if len(job.Lines) > maximumLogLines {
		job.Lines = append([]string{"… sortie antérieure tronquée …"}, job.Lines[len(job.Lines)-maximumLogLines+1:]...)
	}
}

func (s upgradeStore) validate() error {
	info, err := os.Lstat(s.root)
	if err != nil || !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 || info.Mode().Perm()&0007 != 0 {
		return errors.New("insecure update state directory")
	}
	return nil
}

func (s upgradeStore) jobsDirectory() string    { return filepath.Join(s.root, "jobs") }
func (s upgradeStore) jobPath(id string) string { return filepath.Join(s.jobsDirectory(), id+".json") }

func (s upgradeStore) create(job upgradeJob) error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.jobsDirectory(), 0o750); err != nil {
		return err
	}
	if err := s.write(job); err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.root, "current"), []byte(job.ID+"\n"), 0o640)
}

func (s upgradeStore) write(job upgradeJob) error {
	if !validJobID(job.ID) {
		return errors.New("invalid update job id")
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return atomicWrite(s.jobPath(job.ID), append(payload, '\n'), 0o640)
}

func (s upgradeStore) read(id string) (upgradeJob, error) {
	var job upgradeJob
	if !validJobID(id) {
		return job, errors.New("invalid update job id")
	}
	payload, err := os.ReadFile(s.jobPath(id))
	if err != nil {
		return job, err
	}
	if err := json.Unmarshal(payload, &job); err != nil || job.ID != id {
		return upgradeJob{}, errors.New("invalid update job")
	}
	return job, nil
}

func (s upgradeStore) current() (string, error) {
	payload, err := os.ReadFile(filepath.Join(s.root, "current"))
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(payload))
	if !validJobID(id) {
		return "", errors.New("invalid current update job")
	}
	return id, nil
}

func (s upgradeStore) lock() (*os.File, error) {
	file, err := os.OpenFile(filepath.Join(s.root, "updater.lock"), os.O_CREATE|os.O_RDWR, 0o640)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, errors.New("another update is running")
	}
	return file, nil
}

func atomicWrite(path string, payload []byte, mode fs.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".update-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() { _ = os.Remove(name) }()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func validJobID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func newJobID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func installedVersion() string {
	if value, err := os.ReadFile("/usr/share/aegisadmin/VERSION"); err == nil {
		return strings.TrimSpace(string(value))
	}
	return ""
}
