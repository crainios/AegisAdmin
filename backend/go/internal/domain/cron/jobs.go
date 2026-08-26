package cron

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type Job struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Type     string `json:"type"`
	User     string `json:"user"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Line     *int   `json:"line"`
	Enabled  bool   `json:"enabled"`
	Editable bool   `json:"editable"`
	Managed  bool   `json:"managed"`
}

var (
	safeName        = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	environment     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\s*=`)
	macroLine       = regexp.MustCompile(`^@(reboot|yearly|annually|monthly|weekly|daily|midnight|hourly)\b`)
	managedMarker   = regexp.MustCompile(`^# AEGISADMIN-TASK: ([a-f0-9]{8}-[a-f0-9]{4}-[1-5][a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}) (ACTIVE|SUSPENDED)(?: ([A-Za-z0-9_-]+))?$`)
	periodicPath    = regexp.MustCompile(`/etc/cron\.(hourly|daily|weekly|monthly)(?:\s|;|\}|$)`)
	userJobLine     = regexp.MustCompile(`^\s*(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.+)$`)
	systemJobLine   = regexp.MustCompile(`^\s*(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.+)$`)
	userMacroLine   = regexp.MustCompile(`^\s*(\S+)\s+(.+)$`)
	systemMacroLine = regexp.MustCompile(`^\s*(\S+)\s+(\S+)\s+(.+)$`)
	anacronLine     = regexp.MustCompile(`^\s*(\S+)\s+(\S+)\s+(\S+)\s+(.+)$`)
)

type jobParser struct {
	backend  *Backend
	jobs     []Job
	periods  map[string]string
	editable map[string]bool
}

func (b *Backend) jobs() ([]Job, error) {
	users, err := b.users()
	if err != nil {
		return nil, err
	}
	editableUsers := make(map[string]bool, len(users))
	for _, item := range users {
		editableUsers[item.Name] = true
	}
	p := &jobParser{backend: b, jobs: []Job{}, editable: editableUsers, periods: map[string]string{
		"hourly": "/etc/cron.hourly", "daily": "/etc/cron.daily",
		"weekly": "/etc/cron.weekly", "monthly": "/etc/cron.monthly",
	}}
	anacron := executable(b.paths.Anacron) && regularFile(b.paths.Anacrontab)
	if regularFile(b.paths.SystemCrontab) {
		if err := p.parseCrontab(b.paths.SystemCrontab, "system", "", !anacron); err != nil {
			return nil, err
		}
	}
	files, err := safeFiles(b.paths.CronDirectory)
	if err != nil {
		return nil, err
	}
	for _, path := range files {
		if err := p.parseCrontab(path, "system", "", true); err != nil {
			return nil, err
		}
	}
	spools, err := safeFiles(b.paths.SpoolDirectory)
	if err != nil {
		return nil, err
	}
	for _, path := range spools {
		owner := filepath.Base(path)
		if _, err := user.Lookup(owner); err != nil {
			return nil, err
		}
		if err := p.parseCrontab(path, "user", owner, true); err != nil {
			return nil, err
		}
	}
	if anacron {
		if err := p.parseAnacron(); err != nil {
			return nil, err
		}
	}
	sort.Slice(p.jobs, func(i, j int) bool {
		a, z := p.jobs[i], p.jobs[j]
		aline, zline := -1, -1
		if a.Line != nil {
			aline = *a.Line
		}
		if z.Line != nil {
			zline = *z.Line
		}
		left := fmt.Sprintf("%s\x00%s\x00%012d\x00%s\x00%s\x00%s", a.Type, a.Source, aline, a.User, a.Schedule, a.Command)
		right := fmt.Sprintf("%s\x00%s\x00%012d\x00%s\x00%s\x00%s", z.Type, z.Source, zline, z.User, z.Schedule, z.Command)
		return left < right
	})
	return p.jobs, nil
}

func safeFiles(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	paths := []string{}
	for _, entry := range entries {
		if !safeName.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if info.Mode().IsRegular() {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func (p *jobParser) append(source, kind, owner, schedule, command string, line *int, id string, enabled, editable, managed bool) error {
	if owner == "" || schedule == "" || command == "" {
		return errorsText("incomplete cron job")
	}
	if id == "" {
		lineValue := "None"
		if line != nil {
			lineValue = fmt.Sprint(*line)
		}
		id = taskIdentifier("readonly", source, kind, owner, schedule, command, lineValue)
	}
	p.jobs = append(p.jobs, Job{id, source, kind, owner, schedule, command, line, enabled, editable, managed})
	return nil
}

func taskIdentifier(prefix string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("%s-%x", prefix, digest[:16])
}

func legacyTaskIdentifier(owner, schedule, command string, occurrence int) string {
	return taskIdentifier("legacy", owner, normalizeSchedule(schedule), strings.TrimSpace(command), fmt.Sprint(occurrence))
}

func parseJobLine(line, implicitUser string) (string, string, string, error) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "@") {
		if !macroLine.MatchString(trimmed) {
			return "", "", "", errorsText("invalid cron macro")
		}
		if implicitUser == "" {
			parts := systemMacroLine.FindStringSubmatch(line)
			if parts == nil {
				return "", "", "", errorsText("invalid cron macro")
			}
			return parts[1], parts[2], parts[3], nil
		}
		parts := userMacroLine.FindStringSubmatch(line)
		if parts == nil {
			return "", "", "", errorsText("invalid cron macro")
		}
		return parts[1], implicitUser, parts[2], nil
	}
	if implicitUser == "" {
		parts := systemJobLine.FindStringSubmatch(line)
		if parts == nil {
			return "", "", "", errorsText("invalid cron line")
		}
		return strings.Join(parts[1:6], " "), parts[6], parts[7], nil
	}
	parts := userJobLine.FindStringSubmatch(line)
	if parts == nil {
		return "", "", "", errorsText("invalid cron line")
	}
	return strings.Join(parts[1:6], " "), implicitUser, parts[6], nil
}

func (p *jobParser) expandPeriodic(kind, schedule string) error {
	directory := p.periods[kind]
	files, err := safeFiles(directory)
	if err != nil {
		return err
	}
	for _, script := range files {
		if !executable(script) {
			continue
		}
		if err := p.append(script, "periodic", "root", schedule, script, nil, "", true, false, false); err != nil {
			return err
		}
	}
	return nil
}

func (p *jobParser) parseCrontab(path, kind, implicitUser string, expand bool) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !utf8.Valid(content) {
		return errorsText("invalid UTF-8")
	}
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	pending := ""
	legacyOccurrences := map[string]int{}
	for index, raw := range lines {
		lineNumber := index + 1
		line := strings.TrimSpace(raw)
		marker := managedMarker.FindStringSubmatch(line)
		if marker != nil {
			if implicitUser == "" || pending != "" {
				return errorsText("invalid managed task")
			}
			id, state, payload := marker[1], marker[2], marker[3]
			if state == "ACTIVE" {
				if payload != "" {
					return errorsText("invalid active task")
				}
				pending = id
				continue
			}
			if payload == "" {
				return errorsText("invalid suspended task")
			}
			decoded, err := base64.RawURLEncoding.DecodeString(payload)
			if err != nil || len(decoded) == 0 || strings.ContainsAny(string(decoded), "\r\n") {
				return errorsText("invalid suspended task")
			}
			schedule, owner, command, err := parseJobLine(string(decoded), implicitUser)
			if err != nil {
				return err
			}
			lineCopy := lineNumber
			if err := p.append(path, "user", owner, schedule, command, &lineCopy, id, false, p.editable[owner], true); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "# AEGISADMIN-TASK:") {
			return errorsText("invalid managed marker")
		}
		if line == "" || strings.HasPrefix(line, "#") || environment.MatchString(line) {
			continue
		}
		schedule, owner, command, err := parseJobLine(line, implicitUser)
		if err != nil {
			return err
		}
		if match := periodicPath.FindStringSubmatch(command); match != nil && expand {
			if pending != "" {
				return errorsText("managed periodic expansion")
			}
			if err := p.expandPeriodic(match[1], schedule); err != nil {
				return err
			}
			continue
		}
		id, managed := pending, pending != ""
		if id == "" && implicitUser != "" {
			key := normalizeSchedule(schedule) + "\x00" + strings.TrimSpace(command)
			legacyOccurrences[key]++
			id = legacyTaskIdentifier(implicitUser, schedule, command, legacyOccurrences[key])
		}
		lineCopy := lineNumber
		if err := p.append(path, kind, owner, schedule, command, &lineCopy, id, true, kind == "user" && p.editable[owner], managed); err != nil {
			return err
		}
		pending = ""
	}
	if pending != "" {
		return errorsText("orphan managed task")
	}
	return nil
}

func (p *jobParser) parseAnacron() error {
	content, err := os.ReadFile(p.backend.paths.Anacrontab)
	if err != nil {
		return err
	}
	if !utf8.Valid(content) {
		return errorsText("invalid UTF-8")
	}
	for index, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || environment.MatchString(line) {
			continue
		}
		parts := anacronLine.FindStringSubmatch(line)
		if parts == nil {
			return errorsText("invalid anacron line")
		}
		schedule, command := "@anacron "+strings.Join(parts[1:4], " "), parts[4]
		if match := periodicPath.FindStringSubmatch(command); match != nil {
			if err := p.expandPeriodic(match[1], schedule); err != nil {
				return err
			}
			continue
		}
		lineCopy := index + 1
		if err := p.append(p.backend.paths.Anacrontab, "anacron", "root", schedule, command, &lineCopy, "", true, false, false); err != nil {
			return err
		}
	}
	return nil
}

type errorsText string

func (e errorsText) Error() string { return string(e) }
