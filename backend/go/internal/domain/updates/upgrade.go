package updates

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os/exec"
	"strings"
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
	ID         string
	Status     string
	Backend    string
	Lines      []string
	ExitCode   *int
	StartedAt  string
	FinishedAt *string
}

func (b *Backend) startUpgrade() protocol.Reply {
	manager, binary, arguments := "", "", []string{}
	if executable(b.apt) {
		manager, binary, arguments = "apt", b.apt, []string{"-y", "upgrade"}
	} else if executable(b.dnf) {
		manager, binary, arguments = "dnf", b.dnf, []string{"-y", "upgrade"}
	} else {
		return fail(3, "DEPENDENCY_NOT_FOUND", "Aucun gestionnaire de mises à jour APT ou DNF pris en charge n’est installé.")
	}

	b.jobsMu.Lock()
	if b.activeJobID != "" {
		job := b.jobs[b.activeJobID]
		if job != nil && job.Status == "running" {
			id := job.ID
			b.jobsMu.Unlock()
			return protocol.Reply{Response: api.Success(map[string]any{"job_id": id, "already_running": true})}
		}
	}
	id, err := newJobID()
	if err != nil {
		b.jobsMu.Unlock()
		return fail(10, "JOB_ID_GENERATION_FAILED", "Le suivi de la mise à jour n’a pas pu être initialisé.")
	}
	job := &upgradeJob{ID: id, Status: "running", Backend: manager, Lines: []string{"Initialisation de la mise à jour des paquets…"}, StartedAt: time.Now().UTC().Format(time.RFC3339)}
	if b.jobs == nil {
		b.jobs = make(map[string]*upgradeJob)
	}
	b.jobs = map[string]*upgradeJob{id: job}
	b.activeJobID = id
	b.jobsMu.Unlock()

	go b.runUpgrade(job, binary, arguments)
	return protocol.Reply{Response: api.Success(map[string]any{"job_id": id, "already_running": false})}
}

func (b *Backend) runUpgrade(job *upgradeJob, binary string, arguments []string) {
	ctx, cancel := context.WithTimeout(context.Background(), upgradeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, arguments...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C", "DEBIAN_FRONTEND=noninteractive"}
	pipe, err := cmd.StdoutPipe()
	if err == nil {
		cmd.Stderr = cmd.Stdout
		err = cmd.Start()
	}
	if err != nil {
		b.finishUpgrade(job, -1, "Impossible de démarrer le gestionnaire de paquets.")
		return
	}

	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 4096), 256*1024)
	for scanner.Scan() {
		b.appendUpgradeLine(job, scanner.Text())
	}
	if scanErr := scanner.Err(); scanErr != nil {
		b.appendUpgradeLine(job, "La lecture de la sortie du gestionnaire de paquets a été interrompue.")
	}
	err = cmd.Wait()
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}
	if ctx.Err() == context.DeadlineExceeded {
		exitCode = -1
		b.finishUpgrade(job, exitCode, "La mise à jour a dépassé la durée maximale autorisée.")
		return
	}
	message := "Mise à jour des paquets terminée avec succès."
	if exitCode != 0 {
		message = "La mise à jour des paquets s’est terminée avec une erreur."
	}
	b.finishUpgrade(job, exitCode, message)
}

func (b *Backend) appendUpgradeLine(job *upgradeJob, line string) {
	line = strings.TrimSpace(strings.ToValidUTF8(line, "�"))
	if line == "" {
		return
	}
	if len(line) > maximumLineSize {
		line = line[:maximumLineSize] + "…"
	}
	b.jobsMu.Lock()
	defer b.jobsMu.Unlock()
	job.Lines = append(job.Lines, line)
	if len(job.Lines) > maximumLogLines {
		job.Lines = append([]string{"… sortie antérieure tronquée …"}, job.Lines[len(job.Lines)-maximumLogLines+1:]...)
	}
}

func (b *Backend) finishUpgrade(job *upgradeJob, exitCode int, message string) {
	b.jobsMu.Lock()
	defer b.jobsMu.Unlock()
	job.Lines = append(job.Lines, message)
	job.ExitCode = &exitCode
	job.Status = "completed"
	if exitCode != 0 {
		job.Status = "failed"
	}
	finishedAt := time.Now().UTC().Format(time.RFC3339)
	job.FinishedAt = &finishedAt
	b.activeJobID = ""
}

func (b *Backend) upgradeStatus(id string) protocol.Reply {
	if len(id) != 32 {
		return fail(2, "INVALID_JOB_ID", "L’identifiant du travail de mise à jour est invalide.")
	}
	if _, err := hex.DecodeString(id); err != nil {
		return fail(2, "INVALID_JOB_ID", "L’identifiant du travail de mise à jour est invalide.")
	}
	b.jobsMu.Lock()
	defer b.jobsMu.Unlock()
	job := b.jobs[id]
	if job == nil {
		return fail(4, "UPDATE_JOB_NOT_FOUND", "Le travail de mise à jour demandé est introuvable.")
	}
	lines := append([]string(nil), job.Lines...)
	return protocol.Reply{Response: api.Success(map[string]any{"job_id": job.ID, "status": job.Status, "backend": job.Backend, "lines": lines, "exit_code": job.ExitCode, "started_at": job.StartedAt, "finished_at": job.FinishedAt})}
}

func newJobID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
