package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

const defaultDataMount = "/var/www/data"

var findmntCommandCandidates = []string{"/usr/bin/findmnt", "/bin/findmnt", "/usr/local/bin/findmnt"}

var identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var majorMinorPattern = regexp.MustCompile(`^[0-9]+:[0-9]+$`)
var invalidIdentifierCharacters = regexp.MustCompile(`[^a-z0-9._-]`)
var repeatedHyphens = regexp.MustCompile(`-{2,}`)

var excludedFilesystems = map[string]struct{}{
	"autofs": {}, "bpf": {}, "cgroup": {}, "cgroup2": {}, "configfs": {},
	"debugfs": {}, "devpts": {}, "devtmpfs": {}, "efivarfs": {}, "fusectl": {},
	"hugetlbfs": {}, "mqueue": {}, "overlay": {}, "proc": {}, "pstore": {},
	"ramfs": {}, "securityfs": {}, "squashfs": {}, "sysfs": {}, "tmpfs": {}, "tracefs": {},
}

type Mount struct {
	ID             string `json:"id"`
	Source         string `json:"source"`
	Mount          string `json:"mount"`
	Filesystem     string `json:"filesystem"`
	PhysicalDevice string `json:"physical_device"`
	MajorMinor     string `json:"-"`
	Size           int64  `json:"size"`
	Used           int64  `json:"used"`
	Available      int64  `json:"available"`
	Percent        int    `json:"percent"`
}

type Collector interface {
	Mounts(context.Context) ([]Mount, error)
}

type LinuxCollector struct {
	findmnt   string
	dataMount string
}

type Handler struct{ collector Collector }

type findmntPayload struct {
	Filesystems []findmntFilesystem `json:"filesystems"`
}

type findmntFilesystem struct {
	Source     string          `json:"source"`
	Target     string          `json:"target"`
	FSType     string          `json:"fstype"`
	MajorMinor string          `json:"maj:min"`
	Size       json.RawMessage `json:"size"`
	Used       json.RawMessage `json:"used"`
	Avail      json.RawMessage `json:"avail"`
	Use        string          `json:"use%"`
}

func New(collector Collector) *Handler { return &Handler{collector: collector} }
func NewLinuxCollector() *LinuxCollector {
	collector := &LinuxCollector{dataMount: defaultDataMount}
	for _, candidate := range findmntCommandCandidates {
		if executable(candidate) {
			collector.findmnt = candidate
			break
		}
	}
	collector.loadProfile("/etc/aegisadmin-system/storage")
	return collector
}

func (h *Handler) Handle(ctx context.Context, command string, arguments []string) protocol.Reply {
	if command == "" {
		return failure(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine storage.")
	}

	switch command {
	case "list":
		if len(arguments) != 0 {
			return invalidArgumentCount()
		}
		mounts, err := h.collector.Mounts(ctx)
		if err != nil {
			return storageReadFailure(err)
		}
		return success(map[string]any{"mounts": mounts})
	case "status":
		if len(arguments) != 1 {
			return invalidArgumentCount()
		}
		if !identifierPattern.MatchString(arguments[0]) {
			return failure(2, "INVALID_STORAGE_IDENTIFIER", "L’identifiant du point de montage est invalide.")
		}
		mounts, err := h.collector.Mounts(ctx)
		if err != nil {
			return storageReadFailure(err)
		}
		for _, mount := range mounts {
			if mount.ID == arguments[0] {
				payload, _ := json.Marshal(mount)
				data := map[string]any{}
				_ = json.Unmarshal(payload, &data)
				return success(data)
			}
		}
		return failure(5, "STORAGE_NOT_FOUND", "Le point de montage demandé est introuvable.")
	default:
		return failure(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine storage.")
	}
}

func (c *LinuxCollector) Mounts(ctx context.Context) ([]Mount, error) {
	if !executable(c.findmnt) {
		return nil, errors.New("findmnt unavailable")
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, c.findmnt, "--json", "--bytes", "--df", "--output", "SOURCE,TARGET,FSTYPE,MAJ:MIN,SIZE,USED,AVAIL,USE%").Output()
	mounts, normalizeErr := normalizeFindmntResult(output, err, c.dataMount)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	for index := range mounts {
		mounts[index].PhysicalDevice = resolvePhysicalDevices(mounts[index].MajorMinor, "/sys")
	}
	return mounts, nil
}

func normalizeFindmntResult(output []byte, commandErr error, dataMount string) ([]Mount, error) {
	if len(output) == 0 {
		if commandErr != nil {
			return nil, errors.New("findmnt command failed: " + safeCommandError(commandErr))
		}
		return nil, errors.New("findmnt returned no output")
	}
	mounts, err := normalizeWithDataMount(output, dataMount)
	if err != nil {
		return nil, err
	}
	if commandErr != nil && len(mounts) == 0 {
		return nil, errors.New("findmnt failed without usable mounts")
	}
	return mounts, nil
}

func safeCommandError(err error) string {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		message := strings.TrimSpace(strings.ToValidUTF8(string(exitError.Stderr), "�"))
		if message != "" {
			if len(message) > 240 {
				message = message[:240]
			}
			return "exit code " + strconv.Itoa(exitError.ExitCode()) + ": " + message
		}
		return "exit code " + strconv.Itoa(exitError.ExitCode())
	}
	return err.Error()
}

func normalize(raw []byte) ([]Mount, error) {
	return normalizeWithDataMount(raw, defaultDataMount)
}

func normalizeWithDataMount(raw []byte, dataMount string) ([]Mount, error) {
	var payload findmntPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}

	mounts := make([]Mount, 0, len(payload.Filesystems))
	mountIndexes := map[string]int{}
	for _, item := range payload.Filesystems {
		if _, excluded := excludedFilesystems[item.FSType]; excluded || item.Source == "" || item.Target == "" || item.FSType == "" {
			continue
		}
		size, sizeOK := nonNegativeInteger(item.Size)
		used, usedOK := nonNegativeInteger(item.Used)
		available, availableOK := nonNegativeInteger(item.Avail)
		if !sizeOK || !usedOK || !availableOK {
			continue
		}
		candidate := identifierCandidate(item.Target, dataMount)
		if !identifierPattern.MatchString(candidate) {
			continue
		}
		mount := Mount{ID: candidate, Source: item.Source, Mount: item.Target, Filesystem: item.FSType, MajorMinor: item.MajorMinor, Size: size, Used: used, Available: available, Percent: normalizePercent(item.Use)}
		if index, duplicate := mountIndexes[item.Target]; duplicate {
			mounts[index] = mount
			continue
		}
		mountIndexes[item.Target] = len(mounts)
		mounts = append(mounts, mount)
	}

	counts := map[string]int{}
	for _, mount := range mounts {
		counts[mount.ID]++
	}
	seen := map[string]struct{}{}
	for index := range mounts {
		requiresSuffix := counts[mounts[index].ID] > 1
		if mounts[index].ID == "root" {
			requiresSuffix = mounts[index].Mount != "/"
		} else if mounts[index].ID == "data" {
			requiresSuffix = mounts[index].Mount != dataMount
		}
		if requiresSuffix {
			digest := sha256.Sum256([]byte(mounts[index].Mount))
			mounts[index].ID += "-" + hex.EncodeToString(digest[:])[:12]
		}
		if _, exists := seen[mounts[index].ID]; exists {
			return nil, errors.New("storage identifier collision")
		}
		seen[mounts[index].ID] = struct{}{}
	}
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].ID < mounts[j].ID })
	return mounts, nil
}

func resolvePhysicalDevices(majorMinor, sysRoot string) string {
	if !majorMinorPattern.MatchString(majorMinor) {
		return ""
	}
	path, err := filepath.EvalSymlinks(filepath.Join(sysRoot, "dev", "block", majorMinor))
	if err != nil {
		return ""
	}
	devices := map[string]struct{}{}
	collectPhysicalDevices(path, devices, map[string]struct{}{})
	names := make([]string, 0, len(devices))
	for name := range devices {
		names = append(names, "/dev/"+name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func collectPhysicalDevices(path string, devices, visited map[string]struct{}) {
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return
	}
	if _, found := visited[realPath]; found {
		return
	}
	visited[realPath] = struct{}{}
	if _, err := os.Stat(filepath.Join(realPath, "partition")); err == nil {
		collectPhysicalDevices(filepath.Dir(realPath), devices, visited)
		return
	}
	slaves, _ := os.ReadDir(filepath.Join(realPath, "slaves"))
	if len(slaves) > 0 {
		for _, slave := range slaves {
			collectPhysicalDevices(filepath.Join(realPath, "slaves", slave.Name()), devices, visited)
		}
		return
	}
	name := filepath.Base(realPath)
	if name != "" && name != "." {
		devices[name] = struct{}{}
	}
}

func identifierCandidate(mount, dataMount string) string {
	if mount == "/" {
		return "root"
	}
	if dataMount != "" && mount == dataMount {
		return "data"
	}
	identifier := strings.ToLower(strings.TrimLeft(mount, "/"))
	identifier = invalidIdentifierCharacters.ReplaceAllString(identifier, "-")
	identifier = repeatedHyphens.ReplaceAllString(identifier, "-")
	identifier = strings.Trim(identifier, "-")
	if identifier == "" {
		return "mount"
	}
	return identifier
}

func executable(path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0
}

func nonNegativeInteger(raw json.RawMessage) (int64, bool) {
	value, err := strconv.ParseInt(string(raw), 10, 64)
	return value, err == nil && value >= 0
}

func normalizePercent(raw string) int {
	value, err := strconv.Atoi(strings.TrimSuffix(raw, "%"))
	if err != nil || value < 0 || value > 100 {
		return 0
	}
	return value
}

func success(data map[string]any) protocol.Reply { return protocol.Reply{Response: api.Success(data)} }
func failure(exit int, code, message string) protocol.Reply {
	return protocol.Reply{ExitCode: exit, Response: api.Failure(code, message)}
}
func storageReadFailure(err error) protocol.Reply {
	response := api.Failure("STORAGE_READ_FAILED", "Les informations de stockage n’ont pas pu être lues.")
	response.Error.Details = map[string]any{"reason": err.Error()}
	return protocol.Reply{ExitCode: 3, Response: response}
}
func invalidArgumentCount() protocol.Reply {
	return failure(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
}
