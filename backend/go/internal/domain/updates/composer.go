package updates

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const composerProfileFile = "/etc/aegisadmin-system/composer"

var composerVHostPattern = regexp.MustCompile(`(?i)^(?:port\s+\d+\s+namevhost|default server)\s+(\S+)\s+\((/.+\.conf):(\d+)\)$`)
var composerDomainPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?$`)
var composerDocumentRootPattern = regexp.MustCompile(`(?i)^DocumentRoot\s+["']?([^"']+)["']?$`)
var composerEmptyAdvisoriesPattern = regexp.MustCompile(`(?s)"advisories"\s*:\s*\[\s*\]`)

type composerProfile struct {
	enabled       bool
	interval      time.Duration
	composer      string
	apacheControl string
	roots         []string
	maxParents    int
	rootUser      string
}

type composerPackage struct {
	Name    string `json:"name"`
	Current string `json:"current_version"`
	Latest  string `json:"latest_version"`
	Status  string `json:"status"`
	Direct  bool   `json:"direct"`
}

type composerAdvisory struct {
	Package          string `json:"package"`
	ID               string `json:"id"`
	Title            string `json:"title"`
	AffectedVersions string `json:"affected_versions"`
	Link             string `json:"link"`
}

type composerSite struct {
	Domain             string             `json:"domain"`
	ProjectID          string             `json:"project_id"`
	Status             string             `json:"status"`
	UpdateCount        int                `json:"update_count"`
	CompatibleCount    int                `json:"compatible_update_count"`
	SecurityIssueCount int                `json:"security_issue_count"`
	SecurityStatus     string             `json:"security_status"`
	SecurityMessage    string             `json:"security_message"`
	Packages           []composerPackage  `json:"packages"`
	SecurityAdvisories []composerAdvisory `json:"security_advisories"`
	Message            string             `json:"message"`
	Stale              bool               `json:"stale"`
}

type composerSnapshot struct {
	Available   bool           `json:"available"`
	Refreshing  bool           `json:"refreshing"`
	RefreshedAt *string        `json:"refreshed_at"`
	Sites       []composerSite `json:"sites"`
	Message     string         `json:"message"`
}

type composerMonitor struct {
	profile    composerProfile
	runner     Runner
	mu         sync.Mutex
	snapshot   composerSnapshot
	refreshing bool
}

func newComposerMonitor(runner Runner) *composerMonitor {
	profile := loadComposerProfile(composerProfileFile)
	available := profile.enabled && resolveComposer(profile.composer) != ""
	message := ""
	if profile.enabled && !available {
		message = "Composer est introuvable sur le serveur."
	}
	return &composerMonitor{profile: profile, runner: runner, snapshot: composerSnapshot{Available: available, Sites: []composerSite{}, Message: message}}
}

func (monitor *composerMonitor) start(ctx context.Context) {
	if !monitor.profile.enabled {
		monitor.snapshot.Message = "La surveillance Composer est désactivée."
		return
	}
	monitor.requestRefresh()
	go func() {
		ticker := time.NewTicker(monitor.profile.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				monitor.requestRefresh()
			}
		}
	}()
}

func (monitor *composerMonitor) requestRefresh() bool {
	monitor.mu.Lock()
	if monitor.refreshing {
		monitor.mu.Unlock()
		return false
	}
	monitor.refreshing = true
	monitor.snapshot.Refreshing = true
	monitor.mu.Unlock()
	go monitor.refresh()
	return true
}

func (monitor *composerMonitor) current() composerSnapshot {
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	result := monitor.snapshot
	result.Refreshing = monitor.refreshing
	result.Sites = append([]composerSite(nil), monitor.snapshot.Sites...)
	return result
}

func (monitor *composerMonitor) refresh() {
	snapshot := monitor.collect()
	monitor.mu.Lock()
	previous := map[string]composerSite{}
	for _, site := range monitor.snapshot.Sites {
		previous[site.Domain+"\x00"+site.ProjectID] = site
	}
	if len(snapshot.Sites) == 0 && len(previous) > 0 && snapshot.Message != "Aucun projet Composer n’a été détecté parmi les VirtualHosts actifs." {
		snapshot.Sites = append([]composerSite(nil), monitor.snapshot.Sites...)
		snapshot.Available = monitor.snapshot.Available
		snapshot.RefreshedAt = monitor.snapshot.RefreshedAt
		for index := range snapshot.Sites {
			snapshot.Sites[index].Stale = true
			snapshot.Sites[index].Message = "Dernier résultat conservé : " + snapshot.Message
		}
	} else {
		for index, site := range snapshot.Sites {
			old, found := previous[site.Domain+"\x00"+site.ProjectID]
			if found && site.Status == "error" && old.Status != "error" {
				old.Stale = true
				old.Message = "Dernier résultat conservé : " + site.Message
				snapshot.Sites[index] = old
			}
		}
	}
	monitor.refreshing = false
	snapshot.Refreshing = false
	monitor.snapshot = snapshot
	monitor.mu.Unlock()
}

func (monitor *composerMonitor) collect() composerSnapshot {
	composer := resolveComposer(monitor.profile.composer)
	if composer == "" {
		return composerSnapshot{Sites: []composerSite{}, Message: "Composer est introuvable sur le serveur."}
	}
	projects, discoveryMessage := monitor.discoverProjects()
	sites := []composerSite{}
	projectDomains := map[string][]string{}
	for domain, project := range projects {
		projectDomains[project] = append(projectDomains[project], domain)
	}
	for project, domains := range projectDomains {
		result := inspectComposerProject(composer, project, monitor.profile.rootUser)
		sort.Strings(domains)
		for _, domain := range domains {
			copy := result
			copy.Domain = domain
			sites = append(sites, copy)
		}
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].Domain < sites[j].Domain })
	now := time.Now().UTC().Format(time.RFC3339)
	message := discoveryMessage
	if message == "" && len(sites) == 0 {
		message = "Aucun projet Composer n’a été détecté parmi les VirtualHosts actifs."
	}
	return composerSnapshot{Available: true, RefreshedAt: &now, Sites: sites, Message: message}
}

func (monitor *composerMonitor) discoverProjects() (map[string]string, string) {
	output, status := monitor.runner.Run(context.Background(), monitor.profile.apacheControl, "-S")
	if status != 0 {
		return map[string]string{}, "La liste des VirtualHosts Apache est indisponible."
	}
	projects := map[string]string{}
	for _, raw := range strings.Split(output, "\n") {
		match := composerVHostPattern.FindStringSubmatch(strings.TrimSpace(raw))
		if match == nil || !validComposerDomain(match[1]) {
			continue
		}
		line, _ := strconv.Atoi(match[3])
		documentRoot, ok := composerDocumentRoot(match[2], line)
		if !ok {
			continue
		}
		project, ok := findComposerProject(documentRoot, monitor.profile.roots, monitor.profile.maxParents)
		if ok {
			projects[strings.ToLower(match[1])] = project
		}
	}
	return projects, ""
}

func inspectComposerProject(composer, project, rootUser string) composerSite {
	result := composerSite{ProjectID: composerProjectID(project), Status: "error", SecurityStatus: "error", Packages: []composerPackage{}, SecurityAdvisories: []composerAdvisory{}}
	info, err := os.Stat(filepath.Join(project, "composer.lock"))
	if err != nil {
		result.Message = "Le fichier composer.lock n’est pas lisible."
		return result
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		result.Message = "Le propriétaire du projet ne peut pas être déterminé."
		return result
	}
	uid, gid := stat.Uid, stat.Gid
	if uid == 0 {
		uid, gid, ok = resolveRootProjectUser(rootUser)
		if !ok {
			result.Message = "Aucune identité non-root n’est disponible pour analyser ce projet appartenant à root."
			return result
		}
	}
	_, err = user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		result.Message = "Le propriétaire du projet est introuvable."
		return result
	}
	composerHome := "/tmp/aegisadmin-composer-" + strconv.FormatUint(uint64(uid), 10)
	output, commandErr, timedOut := runComposer(
		composer,
		project,
		uid,
		gid,
		composerHome,
		[]string{"outdated", "--locked", "--no-dev", "--format=json"},
		true,
	)
	if commandErr != nil {
		result.Message = composerFailureMessage(output, commandErr, timedOut, uid, gid)
		return result
	}
	packages, parseErr := parseComposerOutdated(output)
	if parseErr != nil {
		result.Message = "La réponse JSON de Composer est invalide."
		return result
	}
	for _, item := range packages {
		result.Packages = append(result.Packages, item)
		if item.Status == "semver-safe-update" {
			result.CompatibleCount++
		}
	}
	result.UpdateCount = len(result.Packages)
	result.SecurityAdvisories, result.SecurityStatus, result.SecurityMessage = composerAudit(composer, project, uid, gid, composerHome)
	result.SecurityIssueCount = len(result.SecurityAdvisories)
	result.Status = "current"
	result.Message = "Les dépendances sont à jour."
	if result.UpdateCount > 0 {
		result.Status = "outdated"
		result.Message = "Des mises à jour Composer sont disponibles."
	}
	return result
}

func parseComposerOutdated(output []byte) ([]composerPackage, error) {
	if composerEmptyArray(output) {
		return []composerPackage{}, nil
	}
	var payload struct {
		Installed []struct {
			Name             string `json:"name"`
			Version          string `json:"version"`
			Latest           string `json:"latest"`
			LatestStatus     string `json:"latest-status"`
			DirectDependency bool   `json:"direct-dependency"`
		} `json:"installed"`
	}
	if err := decodeComposerJSON(output, &payload); err != nil {
		return nil, err
	}
	packages := []composerPackage{}
	for _, item := range payload.Installed {
		if item.Name == "" || item.Version == "" || item.Latest == "" || item.Version == item.Latest {
			continue
		}
		packages = append(packages, composerPackage{Name: item.Name, Current: item.Version, Latest: item.Latest, Status: item.LatestStatus, Direct: item.DirectDependency})
	}
	return packages, nil
}

func composerAudit(composer, project string, uid, gid uint32, home string) ([]composerAdvisory, string, string) {
	output, err, timedOut := runComposer(
		composer,
		project,
		uid,
		gid,
		home,
		[]string{"audit", "--locked", "--no-dev", "--abandoned=ignore", "--format=json"},
		true,
	)
	var exitError *exec.ExitError
	if err != nil && !errors.As(err, &exitError) {
		return []composerAdvisory{}, "error", composerFailureMessage(output, err, timedOut, uid, gid)
	}
	result, parseErr := parseComposerAudit(output)
	if parseErr != nil {
		message := "La réponse JSON de l’audit Composer est invalide : " + parseErr.Error()
		if err != nil {
			message += " (code " + strconv.Itoa(exitError.ExitCode()) + ")"
		}
		return []composerAdvisory{}, "error", message
	}
	return result, "ok", ""
}

func parseComposerAudit(output []byte) ([]composerAdvisory, error) {
	if composerEmptyArray(output) {
		return []composerAdvisory{}, nil
	}
	var payload struct {
		Advisories json.RawMessage `json:"advisories"`
	}
	if err := decodeComposerJSON(output, &payload); err != nil {
		if composerEmptyAdvisoriesPattern.Match(output) {
			return []composerAdvisory{}, nil
		}
		return nil, err
	}
	if len(payload.Advisories) == 0 || bytes.Equal(bytes.TrimSpace(payload.Advisories), []byte("[]")) || bytes.Equal(bytes.TrimSpace(payload.Advisories), []byte("null")) {
		return []composerAdvisory{}, nil
	}
	var advisoriesByPackage map[string][]struct {
		AdvisoryID       string `json:"advisoryId"`
		PackageName      string `json:"packageName"`
		RemoteID         string `json:"remoteId"`
		Title            string `json:"title"`
		Link             string `json:"link"`
		AffectedVersions string `json:"affectedVersions"`
	}
	if err := json.Unmarshal(payload.Advisories, &advisoriesByPackage); err != nil {
		return nil, err
	}
	result := []composerAdvisory{}
	for packageName, advisories := range advisoriesByPackage {
		for _, advisory := range advisories {
			name := advisory.PackageName
			if name == "" {
				name = packageName
			}
			id := advisory.AdvisoryID
			if id == "" {
				id = advisory.RemoteID
			}
			result = append(result, composerAdvisory{Package: name, ID: id, Title: advisory.Title, AffectedVersions: advisory.AffectedVersions, Link: advisory.Link})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Package == result[j].Package {
			return result[i].ID < result[j].ID
		}
		return result[i].Package < result[j].Package
	})
	return result, nil
}

func composerEmptyArray(output []byte) bool {
	return bytes.Equal(bytes.TrimSpace(output), []byte("[]"))
}

func runComposer(composer, project string, uid, gid uint32, home string, arguments []string, retryWithoutLocked bool) ([]byte, error, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	global := []string{"--no-interaction", "--no-plugins", "--no-scripts", "--no-ansi"}
	executable, commandArguments := composerInvocation(resolvePHP(), composer, append(global, arguments...))
	command := exec.CommandContext(ctx, executable, commandArguments...)
	command.Dir = project
	command.Env = composerEnvironment(home)
	command.SysProcAttr = composerCredentials(uid, gid)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	diagnostic := append(append([]byte(nil), output...), stderr.Bytes()...)
	if err != nil && retryWithoutLocked {
		unsupported := ""
		switch {
		case composerRejectsLocked(diagnostic):
			unsupported = "--locked"
		case composerRejectsAbandonedMode(diagnostic):
			unsupported = "--abandoned=ignore"
		}
		if unsupported != "" {
			compatible := make([]string, 0, len(arguments)-1)
			for _, argument := range arguments {
				if argument != unsupported {
					compatible = append(compatible, argument)
				}
			}
			return runComposer(composer, project, uid, gid, home, compatible, true)
		}
	}
	if err != nil {
		return diagnostic, err, ctx.Err() == context.DeadlineExceeded
	}
	return output, nil, false
}

func decodeComposerJSON(output []byte, target any) error {
	if err := json.Unmarshal(output, target); err == nil {
		return nil
	}
	var lastError error
	for start := bytes.IndexByte(output, '{'); start >= 0; {
		decoder := json.NewDecoder(bytes.NewReader(output[start:]))
		if err := decoder.Decode(target); err == nil {
			return nil
		} else {
			lastError = err
		}
		next := bytes.IndexByte(output[start+1:], '{')
		if next < 0 {
			break
		}
		start += next + 1
	}
	if lastError != nil {
		return lastError
	}
	return errors.New("composer output does not contain a JSON object")
}

func composerRejectsLocked(output []byte) bool {
	message := strings.ToLower(string(output))
	return strings.Contains(message, "\"--locked\" option does not exist") ||
		strings.Contains(message, "option \"--locked\" does not exist") ||
		strings.Contains(message, "option --locked does not exist") ||
		strings.Contains(message, "l'option \"--locked\" n'existe pas")
}

func composerRejectsAbandonedMode(output []byte) bool {
	message := strings.ToLower(string(output))
	return strings.Contains(message, "--abandoned") &&
		(strings.Contains(message, "option does not exist") || strings.Contains(message, "option n'existe pas") || strings.Contains(message, "does not accept a value"))
}

func composerFailureMessage(output []byte, err error, timedOut bool, uid, gid uint32) string {
	if timedOut {
		return "La vérification Composer a dépassé le délai de 90 secondes."
	}
	message := strings.ToLower(strings.ToValidUTF8(string(output), "�") + " " + err.Error())
	switch {
	case strings.Contains(message, "permission denied"), strings.Contains(message, "permission non accordée"):
		return "Composer ne peut pas lire le projet avec l’identité non-root configurée."
	case strings.Contains(message, "operation not permitted"):
		return composerIdentityFailureMessage(uid, gid)
	case strings.Contains(message, "allocation of jit memory failed"):
		return "PHP n’a pas pu désactiver son JIT PCRE sous le confinement systemd du daemon."
	case strings.Contains(message, "could not resolve host"), strings.Contains(message, "network is unreachable"), strings.Contains(message, "failed to open stream"):
		return "Composer ne peut pas joindre le dépôt de dépendances."
	case strings.Contains(message, "authentication required"), strings.Contains(message, "invalid credentials"):
		return "Composer ne dispose pas des identifiants requis pour un dépôt privé."
	case strings.Contains(message, "composer.lock") && strings.Contains(message, "not found"):
		return "Composer ne trouve pas le fichier composer.lock du projet."
	case strings.Contains(message, "option") && strings.Contains(message, "does not exist"):
		return "La version de Composer installée ne prend pas en charge une option de surveillance requise."
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return "Composer a interrompu la vérification avec le code " + strconv.Itoa(exitError.ExitCode()) + "."
	}
	return "Le processus Composer n’a pas pu être démarré."
}

func composerInvocation(php, composer string, arguments []string) (string, []string) {
	if php == "" {
		return composer, arguments
	}
	phpArguments := []string{"-d", "pcre.jit=0", composer}
	return php, append(phpArguments, arguments...)
}

func composerIdentityFailureMessage(uid, gid uint32) string {
	if os.Geteuid() != 0 {
		return "Le daemon doit être exécuté par root pour démarrer Composer sous l’identité du projet."
	}
	if err := runCredentialProbe(uint32(os.Geteuid()), gid); err != nil {
		return "Le service systemd du daemon ne conserve pas la capacité CAP_SETGID requise pour démarrer Composer."
	}
	if err := runCredentialProbe(uid, uint32(os.Getegid())); err != nil {
		return "Le service systemd du daemon ne conserve pas la capacité CAP_SETUID requise pour démarrer Composer."
	}
	return "Le confinement du service systemd interdit le démarrage de Composer sous l’identité non-root configurée."
}

func runCredentialProbe(uid, gid uint32) error {
	command := exec.Command("/usr/bin/true")
	command.SysProcAttr = composerCredentials(uid, gid)
	return command.Run()
}

func composerEnvironment(home string) []string {
	return []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"LANG=C",
		"LC_ALL=C",
		"HOME=" + home,
		"COMPOSER_HOME=" + home,
		"COMPOSER_CACHE_DIR=" + home + "/cache",
		"COMPOSER_ALLOW_SUPERUSER=0",
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=url.https://github.com/.insteadOf",
		"GIT_CONFIG_VALUE_0=git@github.com:",
		"GIT_CONFIG_KEY_1=url.https://github.com/.insteadOf",
		"GIT_CONFIG_VALUE_1=ssh://git@github.com/",
	}
}

func composerCredentials(uid, gid uint32) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid:         uid,
			Gid:         gid,
			NoSetGroups: true,
		},
	}
}

func (b *Backend) composerSnapshot() map[string]any {
	if b.composer == nil {
		return map[string]any{"available": false, "refreshing": false, "refreshed_at": nil, "sites": []composerSite{}, "message": "La surveillance Composer n’est pas initialisée."}
	}
	snapshot := b.composer.current()
	encoded, _ := json.Marshal(snapshot)
	result := map[string]any{}
	_ = json.Unmarshal(encoded, &result)
	return result
}

func (b *Backend) refreshComposer() map[string]any {
	if b.composer == nil || !b.composer.profile.enabled {
		return map[string]any{"started": false, "message": "La surveillance Composer n’est pas disponible."}
	}
	started := b.composer.requestRefresh()
	message := "La vérification Composer a démarré."
	if !started {
		message = "Une vérification Composer est déjà en cours."
	}
	return map[string]any{"started": started, "message": message}
}

func loadComposerProfile(path string) composerProfile {
	profile := composerProfile{enabled: true, interval: 6 * time.Hour, composer: "auto", apacheControl: "auto", roots: []string{"/var/www", "/srv/www"}, maxParents: 6, rootUser: "auto"}
	file, err := os.Open(path)
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			switch strings.TrimSpace(key) {
			case "enabled":
				profile.enabled = strings.TrimSpace(value) != "false"
			case "interval":
				if duration, parseErr := time.ParseDuration(strings.TrimSpace(value)); parseErr == nil && duration >= 15*time.Minute {
					profile.interval = duration
				}
			case "composer":
				profile.composer = strings.TrimSpace(value)
			case "apache_control":
				profile.apacheControl = strings.TrimSpace(value)
			case "roots":
				profile.roots = strings.Split(strings.TrimSpace(value), ",")
			case "max_parents":
				if number, parseErr := strconv.Atoi(strings.TrimSpace(value)); parseErr == nil && number >= 0 && number <= 12 {
					profile.maxParents = number
				}
			case "root_project_user":
				profile.rootUser = strings.TrimSpace(value)
			}
		}
	}
	if profile.apacheControl == "auto" || profile.apacheControl == "" {
		profile.apacheControl = profileValue("/etc/aegisadmin-system/apache", "control", "/usr/sbin/apachectl")
	}
	return profile
}

func resolveRootProjectUser(configured string) (uint32, uint32, bool) {
	login := configured
	if configured == "" || configured == "auto" {
		content, err := os.ReadFile("/etc/group")
		if err != nil {
			return 0, 0, false
		}
		for _, line := range strings.Split(string(content), "\n") {
			fields := strings.Split(line, ":")
			if len(fields) == 4 && fields[0] == "aegisadmin-web" {
				members := strings.Split(fields[3], ",")
				sort.Strings(members)
				for _, member := range members {
					if strings.TrimSpace(member) != "" {
						login = strings.TrimSpace(member)
						break
					}
				}
				break
			}
		}
	}
	account, err := user.Lookup(login)
	if err != nil {
		return 0, 0, false
	}
	uid64, uidErr := strconv.ParseUint(account.Uid, 10, 32)
	gid64, gidErr := strconv.ParseUint(account.Gid, 10, 32)
	if uidErr != nil || gidErr != nil || uid64 == 0 {
		return 0, 0, false
	}
	return uint32(uid64), uint32(gid64), true
}

func profileValue(path, wanted, fallback string) string {
	file, err := os.Open(path)
	if err != nil {
		return fallback
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if ok && strings.TrimSpace(key) == wanted {
			value = strings.TrimSpace(value)
			if filepath.IsAbs(value) {
				return value
			}
		}
	}
	return fallback
}

func resolveComposer(configured string) string {
	candidates := []string{configured}
	if configured == "auto" || configured == "" {
		candidates = []string{"/usr/local/bin/composer", "/usr/bin/composer"}
	}
	for _, candidate := range candidates {
		if (candidate == "/usr/bin/composer" || candidate == "/usr/local/bin/composer") && executable(candidate) {
			return candidate
		}
	}
	return ""
}

func resolvePHP() string {
	for _, candidate := range []string{"/usr/bin/php", "/usr/local/bin/php"} {
		if executable(candidate) {
			return candidate
		}
	}
	return ""
}

func validComposerDomain(domain string) bool {
	return len(domain) <= 253 && composerDomainPattern.MatchString(domain)
}

func composerProjectID(path string) string {
	digest := sha256.Sum256([]byte(path))
	return hex.EncodeToString(digest[:])
}

func findComposerProject(documentRoot string, roots []string, maxParents int) (string, bool) {
	current, err := filepath.EvalSymlinks(documentRoot)
	if err != nil {
		return "", false
	}
	allowed := false
	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root != "." && (current == root || strings.HasPrefix(current, root+string(os.PathSeparator))) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", false
	}
	for level := 0; level <= maxParents; level++ {
		if regularFile(filepath.Join(current, "composer.json")) && regularFile(filepath.Join(current, "composer.lock")) {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", false
}

func composerDocumentRoot(path string, virtualHostLine int) (string, bool) {
	if !filepath.IsAbs(path) {
		return "", false
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	lines := []string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 262144)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	start := virtualHostLine - 1
	if start < 0 {
		start = 0
	}
	if start >= len(lines) {
		return "", false
	}
	inside := false
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "<virtualhost") {
			inside = true
			continue
		}
		if inside && strings.HasPrefix(lower, "</virtualhost>") {
			break
		}
		if inside {
			match := composerDocumentRootPattern.FindStringSubmatch(trimmed)
			if match != nil {
				root := strings.TrimRight(strings.TrimSpace(match[1]), "/")
				if root == "" {
					root = "/"
				}
				return root, filepath.IsAbs(root)
			}
		}
	}
	return "", false
}
