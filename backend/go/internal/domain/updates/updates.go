package updates

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

const (
	aptCommand         = "/usr/bin/apt"
	aptGetCommand      = "/usr/bin/apt-get"
	dnfCommand         = "/usr/bin/dnf"
	rpmCommand         = "/usr/bin/rpm"
	fwupdCommand       = "/usr/bin/fwupdmgr"
	aptListsDirectory  = "/var/lib/apt/lists"
	dnfCacheDirectory  = "/var/cache/dnf"
	rebootRequiredFile = "/var/run/reboot-required"
	systemctlCommand   = "/usr/bin/systemctl"
	systemdRunCommand  = "/usr/bin/systemd-run"
	updateStatePath    = "/var/lib/aegisadmin/updates"
)

type Runner interface {
	Run(context.Context, string, ...string) (string, int)
}

type Backend struct {
	runner                                     Runner
	apt, dnf, rpm, fwupd                       string
	aptLists, dnfCache, rebootFile             string
	upgradeMu                                  sync.Mutex
	aptGet, systemctl, systemdRun, updateState string
	composer                                   *composerMonitor
}

type Handler struct{ backend *Backend }
type execRunner struct{}

type Update struct {
	Name             string `json:"name"`
	Architecture     string `json:"architecture"`
	InstalledVersion string `json:"installed_version"`
	CandidateVersion string `json:"candidate_version"`
	Security         bool   `json:"security"`
}

type FirmwareUpdate struct {
	Device           string   `json:"device"`
	Vendor           string   `json:"vendor"`
	InstalledVersion string   `json:"installed_version"`
	CandidateVersion string   `json:"candidate_version"`
	Release          string   `json:"release"`
	Summary          string   `json:"summary"`
	Remote           string   `json:"remote"`
	RebootRequired   bool     `json:"reboot_required"`
	Issues           []string `json:"issues"`
}

var aptLine = regexp.MustCompile(`^([^/]+)/([^ ]+)\s+(\S+)\s+(\S+)\s+\[upgradable from: (.+)\]$`)

func New(backend *Backend) *Handler { return &Handler{backend} }
func NewLinuxBackend(ctx context.Context) *Backend {
	backend := &Backend{runner: execRunner{}, apt: aptCommand, aptGet: aptGetCommand, dnf: dnfCommand, rpm: rpmCommand, fwupd: fwupdCommand, aptLists: aptListsDirectory, dnfCache: dnfCacheDirectory, rebootFile: rebootRequiredFile, systemctl: systemctlCommand, systemdRun: systemdRunCommand, updateState: updateStatePath}
	backend.composer = newComposerMonitor(backend.runner)
	backend.composer.start(ctx)
	return backend
}

func (execRunner) Run(ctx context.Context, name string, args ...string) (string, int) {
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, name, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(strings.ToValidUTF8(string(out), "�")), 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return strings.TrimSpace(strings.ToValidUTF8(string(out), "�")), exitError.ExitCode()
	}
	return "", -1
}

func (h *Handler) Handle(ctx context.Context, command string, args []string) protocol.Reply {
	if command == "" {
		return fail(2, "MISSING_COMMAND", "Aucune commande n’a été indiquée pour le domaine updates.")
	}
	if command != "info" && command != "list" && command != "firmware" && command != "upgrade-start" && command != "upgrade-status" && command != "composer-status" && command != "composer-refresh" && command != "reboot" {
		return fail(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine updates.")
	}
	if command == "upgrade-start" {
		if len(args) != 0 {
			return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
		}
		return h.backend.startUpgrade()
	}
	if command == "upgrade-status" {
		if len(args) != 1 {
			return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
		}
		return h.backend.upgradeStatus(args[0])
	}
	if command == "composer-status" {
		if len(args) != 0 {
			return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
		}
		return protocol.Reply{Response: api.Success(h.backend.composerSnapshot())}
	}
	if command == "composer-refresh" {
		if len(args) != 0 {
			return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
		}
		return protocol.Reply{Response: api.Success(h.backend.refreshComposer())}
	}
	if command == "reboot" {
		if len(args) != 1 {
			return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
		}
		return h.backend.scheduleReboot(args[0])
	}
	if len(args) != 0 {
		return fail(2, "INVALID_ARGUMENT_COUNT", "Le nombre d’arguments fourni est invalide.")
	}
	data, failure := h.backend.execute(ctx, command)
	if failure != nil {
		return *failure
	}
	return protocol.Reply{Response: api.Success(data)}
}

func (b *Backend) execute(ctx context.Context, command string) (map[string]any, *protocol.Reply) {
	if command == "firmware" {
		return b.firmware(ctx)
	}
	updates, backend, refresh, failure := b.packageUpdates(ctx)
	if failure != nil {
		return nil, failure
	}
	if command == "list" {
		return map[string]any{"updates": updates, "count": len(updates)}, nil
	}
	security := 0
	for _, update := range updates {
		if update.Security {
			security++
		}
	}
	return map[string]any{
		"backend": backend, "last_refresh": refresh,
		"update_count": len(updates), "security_update_count": security,
		"reboot_required": regularFile(b.rebootFile),
	}, nil
}

func (b *Backend) packageUpdates(ctx context.Context) ([]Update, string, any, *protocol.Reply) {
	if executable(b.apt) {
		if failure := b.requireAPT(); failure != nil {
			return nil, "", nil, failure
		}
		output, status := b.runner.Run(ctx, b.apt, "list", "--upgradable")
		if status != 0 {
			return nil, "", nil, reply(3, "UPDATES_READ_FAILED", "Les informations de mises à jour n’ont pas pu être lues.")
		}
		return parseAPTUpdates(output), "apt", lastRefresh(b.aptLists), nil
	}
	if executable(b.dnf) && executable(b.rpm) {
		updates, failure := b.dnfUpdates(ctx)
		return updates, "dnf", lastRefresh(b.dnfCache), failure
	}
	return nil, "", nil, reply(3, "DEPENDENCY_NOT_FOUND", "Aucun gestionnaire de mises à jour APT ou DNF pris en charge n’est installé.")
}

func (b *Backend) dnfUpdates(ctx context.Context) ([]Update, *protocol.Reply) {
	output, status := b.runner.Run(ctx, b.dnf, "--quiet", "check-update")
	if status != 0 && status != 100 {
		return nil, reply(3, "UPDATES_READ_FAILED", "Les informations de mises à jour n’ont pas pu être lues.")
	}
	updates := parseDNFUpdates(output)
	if len(updates) == 0 {
		return updates, nil
	}
	args := []string{"-q", "--qf", `%{NAME}\t%{ARCH}\t%{EVR}\n`, "--"}
	for _, update := range updates {
		args = append(args, update.Name+"."+update.Architecture)
	}
	installed, rpmStatus := b.runner.Run(ctx, b.rpm, args...)
	if rpmStatus != 0 {
		return nil, reply(3, "UPDATES_READ_FAILED", "Les versions installées n’ont pas pu être lues.")
	}
	versions := map[string]string{}
	for _, line := range strings.Split(installed, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 3 {
			versions[fields[0]+"."+fields[1]] = fields[2]
		}
	}
	securityOutput, securityStatus := b.runner.Run(ctx, b.dnf, "--quiet", "updateinfo", "list", "--security", "--available")
	if securityStatus != 0 {
		return nil, reply(3, "UPDATES_READ_FAILED", "Les informations de sécurité des mises à jour n’ont pas pu être lues.")
	}
	securityText := strings.ToLower(securityOutput)
	for index := range updates {
		key := updates[index].Name + "." + updates[index].Architecture
		version, found := versions[key]
		if !found {
			return nil, reply(3, "UPDATES_READ_FAILED", "Une version installée n’a pas pu être déterminée.")
		}
		updates[index].InstalledVersion = version
		updates[index].Security = strings.Contains(securityText, strings.ToLower(updates[index].Name)+"-") && strings.Contains(securityText, "."+strings.ToLower(updates[index].Architecture))
	}
	return updates, nil
}

func parseDNFUpdates(output string) []Update {
	updates := []Update{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		name, architecture, found := strings.Cut(fields[0], ".")
		if !found || name == "" || architecture == "" {
			continue
		}
		updates = append(updates, Update{Name: name, Architecture: architecture, CandidateVersion: fields[1]})
	}
	sort.Slice(updates, func(i, j int) bool {
		if updates[i].Name == updates[j].Name {
			return updates[i].Architecture < updates[j].Architecture
		}
		return updates[i].Name < updates[j].Name
	})
	return updates
}

func (b *Backend) requireAPT() *protocol.Reply {
	if !executable(b.apt) {
		return reply(3, "DEPENDENCY_NOT_FOUND", "La dépendance apt nécessaire au domaine updates est introuvable.")
	}
	info, err := os.Stat(b.aptLists)
	if os.IsNotExist(err) || err == nil && !info.IsDir() {
		return reply(3, "APT_LISTS_NOT_FOUND", "Le répertoire des index APT est introuvable.")
	}
	if err != nil {
		return reply(3, "APT_LISTS_NOT_READABLE", "Le répertoire des index APT n’est pas lisible.")
	}
	directory, err := os.Open(b.aptLists)
	if err != nil {
		return reply(3, "APT_LISTS_NOT_READABLE", "Le répertoire des index APT n’est pas lisible.")
	}
	_ = directory.Close()
	return nil
}

func parseAPTUpdates(output string) []Update {
	updates := []Update{}
	for _, raw := range strings.Split(output, "\n") {
		match := aptLine.FindStringSubmatch(strings.TrimSpace(raw))
		if match == nil {
			continue
		}
		origin := strings.ToLower(match[2])
		updates = append(updates, Update{
			Name: match[1], Architecture: match[4], InstalledVersion: match[5],
			CandidateVersion: match[3],
			Security:         strings.Contains(origin, "-security") || strings.Contains(origin, "security.ubuntu.com"),
		})
	}
	sort.Slice(updates, func(i, j int) bool {
		if updates[i].Name == updates[j].Name {
			return updates[i].Architecture < updates[j].Architecture
		}
		return updates[i].Name < updates[j].Name
	})
	return updates
}

func lastRefresh(directory string) any {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}
	var latest time.Time
	for _, entry := range entries {
		if entry.Name() == "lock" || entry.Name() == "partial" {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.Mode().IsRegular() && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	if latest.IsZero() {
		return nil
	}
	return latest.UTC().Truncate(time.Second).Format(time.RFC3339)
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0
}

func (b *Backend) firmware(ctx context.Context) (map[string]any, *protocol.Reply) {
	if !executable(b.fwupd) {
		return map[string]any{"backend": "fwupd", "version": nil, "available": false, "device_count": 0, "reboot_required": false, "updates": []FirmwareUpdate{}}, nil
	}
	versionOutput, _ := b.runner.Run(ctx, b.fwupd, "--version")
	payload, status := b.runner.Run(ctx, b.fwupd, "get-upgrades", "--json")
	if status != 0 && status != 2 {
		return nil, reply(3, "FIRMWARE_UPDATES_READ_FAILED", "Les mises à jour de firmware n’ont pas pu être lues.")
	}
	updates, valid := parseFirmware(payload, status)
	if !valid {
		return nil, reply(3, "INVALID_FIRMWARE_UPDATES_RESPONSE", "La réponse de fwupd est invalide.")
	}
	reboot := false
	for _, update := range updates {
		reboot = reboot || update.RebootRequired
	}
	var version any
	if value := fwupdVersion(versionOutput); value != "" {
		version = value
	}
	return map[string]any{"backend": "fwupd", "version": version, "available": true, "device_count": len(updates), "reboot_required": reboot, "updates": updates}, nil
}

func fwupdVersion(output string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "runtime" && fields[1] == "org.freedesktop.fwupd" {
			return fields[len(fields)-1]
		}
	}
	return ""
}

func parseFirmware(output string, status int) ([]FirmwareUpdate, bool) {
	if strings.TrimSpace(output) == "" {
		return []FirmwareUpdate{}, status == 2
	}
	var payload struct{ Devices json.RawMessage }
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return nil, false
	}
	if payload.Devices == nil {
		return []FirmwareUpdate{}, true
	}
	if strings.TrimSpace(string(payload.Devices)) == "null" {
		return nil, false
	}
	var devices []map[string]any
	if err := json.Unmarshal(payload.Devices, &devices); err != nil {
		return nil, false
	}
	updates := []FirmwareUpdate{}
	for _, device := range devices {
		rawReleases, exists := device["Releases"]
		if !exists {
			continue
		}
		releases, ok := rawReleases.([]any)
		if !ok {
			continue
		}
		var selected map[string]any
		for _, raw := range releases {
			release, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if selected == nil {
				selected = release
			}
			if contains(stringList(release["Flags"]), "is-upgrade") {
				selected = release
				break
			}
		}
		if selected == nil || !contains(stringList(selected["Flags"]), "is-upgrade") {
			continue
		}
		updates = append(updates, FirmwareUpdate{
			Device: text(device["Name"]), Vendor: text(device["Vendor"]), InstalledVersion: text(device["Version"]),
			CandidateVersion: text(selected["Version"]), Release: text(selected["Name"]), Summary: text(selected["Summary"]),
			Remote: text(selected["RemoteId"]), Issues: stringList(selected["Issues"]),
			RebootRequired: contains(stringList(device["Flags"]), "needs-reboot") || contains(stringList(selected["Flags"]), "needs-reboot"),
		})
	}
	sort.Slice(updates, func(i, j int) bool {
		left, right := strings.ToLower(updates[i].Device), strings.ToLower(updates[j].Device)
		if left == right {
			return strings.ToLower(updates[i].Vendor) < strings.ToLower(updates[j].Vendor)
		}
		return left < right
	})
	return updates, true
}

func text(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func stringList(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return []string{}
	}
	result := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		value := text(item)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func reply(exitCode int, code, message string) *protocol.Reply {
	result := fail(exitCode, code, message)
	return &result
}

func fail(exitCode int, code, message string) protocol.Reply {
	return protocol.Reply{ExitCode: exitCode, Response: api.Failure(code, message)}
}
