package logs

import (
	"bytes"
	"context"
	"errors"
	"io"
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
	minimumLines = 1
	maximumLines = 5000
	tailCommand  = "/usr/bin/tail"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
var numberedRotationPattern = regexp.MustCompile(`\.[0-9]+$`)
var compressedRotationPattern = regexp.MustCompile(`\.[0-9]+\.(gz|xz|zst|zip|bz2)$`)

var errNotFound = errors.New("log not found")
var errNotReadable = errors.New("log not readable")

type Log struct {
	ID   string `json:"id"`
	Path string `json:"-"`
}

type Collector interface {
	Logs(context.Context) []Log
	Tail(context.Context, string, int) ([]string, int64, error)
}

type commandRunner interface {
	Run(context.Context, ...string) ([]byte, error)
}

type LinuxCollector struct {
	root        string
	directories []string
	files       []string
	runner      commandRunner
}

type execRunner struct{}
type Handler struct{ collector Collector }

func New(collector Collector) *Handler { return &Handler{collector: collector} }

func NewLinuxCollector() *LinuxCollector {
	directories, files := loadPolicy(logsPolicyFile)
	return &LinuxCollector{
		root:        logsRoot,
		directories: directories,
		files:       files,
		runner:      execRunner{},
	}
}

func (h *Handler) Handle(ctx context.Context, command string, arguments []string) protocol.Reply {
	if command == "" {
		return failure(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine logs.")
	}

	switch command {
	case "list":
		if len(arguments) != 0 {
			return invalidArgumentCount()
		}
		logs := h.collector.Logs(ctx)
		items := make([]map[string]any, 0, len(logs))
		for _, log := range logs {
			items = append(items, map[string]any{"id": log.ID})
		}
		return success(map[string]any{"logs": items})

	case "tail":
		if len(arguments) != 2 {
			return invalidArgumentCount()
		}
		if !validIdentifier(arguments[0]) {
			return failure(2, "INVALID_LOG_IDENTIFIER", "L’identifiant du journal est invalide.")
		}
		lines, err := strconv.Atoi(arguments[1])
		if err != nil || lines < minimumLines || lines > maximumLines || !digitsOnly(arguments[1]) {
			return failure(2, "INVALID_LINE_COUNT", "Le nombre de lignes doit être compris entre 1 et 5000.")
		}
		content, total, err := h.collector.Tail(ctx, arguments[0], lines)
		switch {
		case errors.Is(err, errNotFound):
			return failure(5, "LOG_NOT_FOUND", "Le journal demandé est introuvable ou n’est pas autorisé.")
		case errors.Is(err, errNotReadable):
			return failure(7, "LOG_NOT_READABLE", "Le journal demandé ne peut pas être lu.")
		case err != nil:
			return failure(10, "LOG_READ_FAILED", "La lecture du journal a échoué.")
		}
		return success(map[string]any{
			"id":              arguments[0],
			"requested_lines": lines,
			"total_lines":     total,
			"lines":           content,
		})

	default:
		return failure(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine logs.")
	}
}

func (c *LinuxCollector) Logs(context.Context) []Log {
	logs := []Log{}
	for _, directory := range c.directories {
		resolvedDirectory, safe := safeResolvedPath(c.root, directory)
		if !safe {
			continue
		}
		entries, err := os.ReadDir(resolvedDirectory)
		if err != nil {
			continue
		}
		prefix := filepath.Base(resolvedDirectory)
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink != 0 || excludedFilename(entry.Name()) {
				continue
			}
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			identifier := prefix + "/" + entry.Name()
			if validIdentifier(identifier) {
				logs = append(logs, Log{ID: identifier, Path: filepath.Join(resolvedDirectory, entry.Name())})
			}
		}
	}
	for _, path := range c.files {
		resolvedPath, safe := safeResolvedPath(c.root, path)
		if !safe {
			continue
		}
		info, err := os.Lstat(resolvedPath)
		if err != nil || !info.Mode().IsRegular() || excludedFilename(filepath.Base(path)) {
			continue
		}
		identifier := filepath.Base(path)
		if validIdentifier(identifier) {
			logs = append(logs, Log{ID: identifier, Path: resolvedPath})
		}
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].ID < logs[j].ID })
	return logs
}

func (c *LinuxCollector) Tail(ctx context.Context, identifier string, lines int) ([]string, int64, error) {
	path := ""
	for _, log := range c.Logs(ctx) {
		if log.ID == identifier {
			path = log.Path
			break
		}
	}
	if path == "" {
		return nil, 0, errNotFound
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, errNotReadable
	}
	total, err := countLines(file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, 0, errNotReadable
	}
	output, err := c.runner.Run(ctx, "-n", strconv.Itoa(lines), "--", path)
	if err != nil {
		return nil, 0, err
	}
	trimmed := strings.TrimRight(string(output), "\n")
	if trimmed == "" {
		return []string{}, total, nil
	}
	return strings.Split(trimmed, "\n"), total, nil
}

func countLines(reader io.Reader) (int64, error) {
	buffer := make([]byte, 64*1024)
	var total int64
	var last byte
	var readAny bool
	for {
		read, err := reader.Read(buffer)
		if read > 0 {
			readAny = true
			last = buffer[read-1]
			total += int64(bytes.Count(buffer[:read], []byte{'\n'}))
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	if readAny && last != '\n' {
		total++
	}
	return total, nil
}

func (execRunner) Run(ctx context.Context, arguments ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return exec.CommandContext(commandCtx, tailCommand, arguments...).Output()
}

func validIdentifier(identifier string) bool {
	return identifier != "" && !strings.HasPrefix(identifier, "/") &&
		!strings.Contains(identifier, "..") && !strings.Contains(identifier, "//") &&
		identifierPattern.MatchString(identifier)
}

func digitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func excludedFilename(filename string) bool {
	for _, suffix := range []string{".gz", ".xz", ".zst", ".zip", ".bz2", ".old", ".tmp", ".temp", ".swp"} {
		if strings.HasSuffix(filename, suffix) {
			return true
		}
	}
	return numberedRotationPattern.MatchString(filename) || compressedRotationPattern.MatchString(filename)
}

func success(data map[string]any) protocol.Reply { return protocol.Reply{Response: api.Success(data)} }
func failure(exit int, code, message string) protocol.Reply {
	return protocol.Reply{ExitCode: exit, Response: api.Failure(code, message)}
}
func invalidArgumentCount() protocol.Reply {
	return failure(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
}
