package certbot

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"aegisadmin/backend/internal/protocol"
)

var versionPattern = regexp.MustCompile(`^certbot\s+([0-9]+(?:\.[0-9]+){1,3}(?:[-+._A-Za-z0-9]*)?)$`)
var pluginPattern = regexp.MustCompile(`(?m)^\* ([a-z][a-z0-9-]*)$`)
var readCertificateName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,253}$`)
var systemdTime = regexp.MustCompile(`^[A-Z][a-z]{2} (\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}:\d{2}) (?:UTC|[A-Z]{3,5})$`)

func (b *Backend) info(ctx context.Context) (map[string]any, *protocol.Reply) {
	b.infoMu.Lock()
	defer b.infoMu.Unlock()

	if b.infoCache != nil {
		return cloneInfo(b.infoCache), nil
	}

	versionOutput, status := b.runner.Run(ctx, b.paths.Certbot, "--version")
	match := versionPattern.FindStringSubmatch(versionOutput)
	if status != 0 || match == nil {
		return nil, b.commandFailure(status)
	}
	packageOutput, packageStatus := "", -1
	if executable(b.paths.DpkgQuery) {
		packageOutput, packageStatus = b.runner.Run(ctx, b.paths.DpkgQuery, "-W", "-f=${Package}\t${Version}\n", "certbot")
	}
	var pkg, packageVersion any = nil, nil
	installation := "unknown"
	if packageStatus == 0 {
		parts := strings.Split(strings.TrimSpace(packageOutput), "\t")
		if len(parts) != 2 || parts[0] != "certbot" {
			return nil, b.readFailure()
		}
		pkg, packageVersion, installation = parts[0], parts[1], "apt"
	} else if executable(b.paths.RPM) {
		packageOutput, packageStatus = b.runner.Run(ctx, b.paths.RPM, "-qf", "--qf", "%{NAME}\t%{EVR}\\n", b.paths.Certbot)
		if packageStatus == 0 {
			parts := strings.Split(strings.TrimSpace(packageOutput), "\t")
			if len(parts) != 2 || (parts[0] != "certbot" && parts[0] != "python3-certbot") {
				return nil, b.readFailure()
			}
			pkg, packageVersion, installation = parts[0], parts[1], "rpm"
		}
	}
	if installation == "unknown" {
		installation = "manual"
	}
	pluginsOutput, status := b.runner.Run(ctx, b.paths.Certbot, "plugins")
	if status != 0 {
		return nil, b.commandFailure(status)
	}
	seen := map[string]bool{}
	plugins := []string{}
	for _, match := range pluginPattern.FindAllStringSubmatch(pluginsOutput, -1) {
		if !seen[match[1]] {
			seen[match[1]] = true
			plugins = append(plugins, match[1])
		}
	}
	sort.Strings(plugins)
	if len(plugins) == 0 {
		return nil, b.readFailure()
	}
	data := map[string]any{"product": "Certbot", "version": match[1], "executable": b.paths.Certbot, "installation": installation, "package": pkg, "package_version": packageVersion, "plugins": plugins}
	b.infoCache = cloneInfo(data)
	return data, nil
}

func cloneInfo(data map[string]any) map[string]any {
	result := make(map[string]any, len(data))
	for key, value := range data {
		if plugins, ok := value.([]string); ok {
			result[key] = append([]string(nil), plugins...)
			continue
		}
		result[key] = value
	}
	return result
}

func (b *Backend) status(ctx context.Context) (map[string]any, *protocol.Reply) {
	timer, ok := b.properties(ctx, "certbot.timer", []string{"LoadState", "ActiveState", "UnitFileState", "LastTriggerUSec", "NextElapseUSecRealtime"})
	if !ok {
		return nil, b.readFailure()
	}
	service, ok := b.properties(ctx, "certbot.service", []string{"LoadState", "ActiveState", "SubState", "Result", "ExecMainCode", "ExecMainStatus"})
	if !ok {
		return nil, b.readFailure()
	}
	exit, err := strconv.Atoi(service["ExecMainStatus"])
	if err != nil {
		return nil, b.readFailure()
	}
	last, ok := systemdTimestamp(timer["LastTriggerUSec"])
	if !ok {
		return nil, b.readFailure()
	}
	next, ok := systemdTimestamp(timer["NextElapseUSecRealtime"])
	if !ok {
		return nil, b.readFailure()
	}
	return map[string]any{"service": "certbot", "unit": "certbot.service", "timer": "certbot.timer", "exists": service["LoadState"] == "loaded", "timer_exists": timer["LoadState"] == "loaded", "timer_active": timer["ActiveState"] == "active", "timer_enabled": timer["UnitFileState"] == "enabled", "service_active": service["ActiveState"] == "active", "service_state": service["SubState"], "last_result": service["Result"], "last_exit_code": exit, "last_trigger": last, "next_trigger": next}, nil
}
func (b *Backend) properties(ctx context.Context, unit string, names []string) (map[string]string, bool) {
	args := []string{"show", unit}
	for _, name := range names {
		args = append(args, "--property", name)
	}
	output, status := b.runner.Run(ctx, b.paths.Systemctl, args...)
	if status != 0 {
		return nil, false
	}
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = value
		}
	}
	return values, true
}
func systemdTimestamp(value string) (any, bool) {
	if value == "" || value == "n/a" {
		return nil, true
	}
	match := systemdTime.FindStringSubmatch(value)
	if match == nil {
		return nil, false
	}
	return match[1] + "T" + match[2] + "Z", true
}

type Certificate struct {
	Name          string   `json:"name"`
	Serial        string   `json:"serial"`
	KeyType       string   `json:"key_type"`
	Domains       []string `json:"domains"`
	Issuer        string   `json:"issuer"`
	Expiry        string   `json:"expiry"`
	DaysRemaining int      `json:"days_remaining"`
	Valid         bool     `json:"valid"`
	Authenticator any      `json:"authenticator"`
	Installer     any      `json:"installer"`
}

func (b *Backend) certificates(_ context.Context) (map[string]any, *protocol.Reply) {
	info, err := os.Stat(b.paths.RenewalDirectory)
	if err != nil || !info.IsDir() {
		return nil, b.readFailure()
	}
	matches, err := filepath.Glob(filepath.Join(b.paths.RenewalDirectory, "*.conf"))
	if err != nil {
		return nil, b.readFailure()
	}
	sort.Strings(matches)
	items := []Certificate{}
	for _, path := range matches {
		item, err := b.certificate(path)
		if err != nil {
			return nil, b.readFailure()
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Expiry == items[j].Expiry {
			return items[i].Name < items[j].Name
		}
		return items[i].Expiry < items[j].Expiry
	})
	return map[string]any{"certificates": items, "count": len(items)}, nil
}
func (b *Backend) certificate(configPath string) (Certificate, error) {
	name := strings.TrimSuffix(filepath.Base(configPath), filepath.Ext(configPath))
	if !readCertificateName.MatchString(name) {
		return Certificate{}, errorsText("invalid name")
	}
	authenticator, installer, err := renewalParameters(configPath)
	if err != nil {
		return Certificate{}, err
	}
	live := filepath.Join(b.paths.LiveDirectory, name, "cert.pem")
	resolved, err := filepath.EvalSymlinks(live)
	if err != nil {
		return Certificate{}, err
	}
	archive, err := filepath.EvalSymlinks(b.paths.ArchiveDirectory)
	if err != nil {
		return Certificate{}, err
	}
	relative, err := filepath.Rel(archive, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return Certificate{}, errorsText("outside archive")
	}
	content, err := os.ReadFile(live)
	if err != nil {
		return Certificate{}, err
	}
	block, _ := pem.Decode(content)
	if block == nil {
		return Certificate{}, errorsText("invalid PEM")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return Certificate{}, err
	}
	domains := append([]string{}, certificate.DNSNames...)
	sort.Strings(domains)
	domains = unique(domains)
	if len(domains) == 0 {
		return Certificate{}, errorsText("no domain")
	}
	keyType := ""
	switch certificate.PublicKey.(type) {
	case *ecdsa.PublicKey:
		keyType = "ECDSA"
	case *rsa.PublicKey:
		keyType = "RSA"
	case ed25519.PublicKey:
		keyType = "Ed25519"
	default:
		return Certificate{}, errorsText("unsupported key")
	}
	issuer := ""
	if len(certificate.Issuer.Organization) > 0 {
		issuer = certificate.Issuer.Organization[0]
	} else {
		issuer = certificate.Issuer.CommonName
	}
	remaining := certificate.NotAfter.Sub(b.now())
	days := int(math.Floor(remaining.Seconds() / 86400))
	serial := formatSerial(certificate.SerialNumber.Text(16))
	return Certificate{name, serial, keyType, domains, issuer, certificate.NotAfter.UTC().Truncate(time.Second).Format(time.RFC3339), days, remaining > 0, nullable(authenticator), nullable(installer)}, nil
}
func formatSerial(value string) string {
	value = strings.ToLower(value)
	if len(value)%2 != 0 {
		value = "0" + value
	}
	return value
}
func renewalParameters(path string) (string, string, error) {
	content, err := os.ReadFile(path)
	if err != nil || !utf8.Valid(content) {
		return "", "", errorsText("invalid config")
	}
	inside := false
	values := map[string]string{}
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inside = line == "[renewalparams]"
			continue
		}
		if !inside {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return "", "", errorsText("invalid config")
		}
		key = strings.TrimSpace(key)
		if key != "authenticator" && key != "installer" {
			continue
		}
		if _, exists := values[key]; exists {
			return "", "", errorsText("duplicate")
		}
		values[key] = strings.TrimSpace(value)
	}
	auth, ok := values["authenticator"]
	if !ok {
		return "", "", errorsText("missing authenticator")
	}
	return auth, values["installer"], nil
}
func unique(values []string) []string {
	result := []string{}
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func (b *Backend) readFailure() *protocol.Reply {
	return reply(10, "CERTBOT_READ_FAILED", "Les informations Certbot n’ont pas pu être lues.")
}
func (b *Backend) commandFailure(status int) *protocol.Reply {
	if status == -2 {
		return reply(10, "CERTBOT_COMMAND_TIMEOUT", "La commande Certbot a dépassé le délai autorisé.")
	}
	return b.readFailure()
}

type errorsText string

func (e errorsText) Error() string { return string(e) }
