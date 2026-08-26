package apache

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	maximumSiteSize   = 65536
	maximumConfigSize = 262144
)

type LinuxBackend struct {
	runner         commandRunner
	service        string
	apacheBinary   string
	apacheControl  string
	enableCommand  string
	disableCommand string
	sitesAvailable string
	sitesEnabled   string
	backupRoot     string
	apacheRoot     string
}

var modulePattern = regexp.MustCompile(`(?i)^([a-zA-Z0-9_]+)_module\s+\((shared|static)\)$`)
var nameVHostPattern = regexp.MustCompile(`(?i)^port\s+(\d+)\s+namevhost\s+(\S+)\s+\((.+):(\d+)\)$`)
var defaultServerPattern = regexp.MustCompile(`(?i)^default server\s+(\S+)\s+\((.+):(\d+)\)$`)
var configPathPattern = regexp.MustCompile(`(?i)\((/.+\.conf):(\d+)\)$`)
var serverNamePattern = regexp.MustCompile(`(?im)^[ \t]*ServerName[ \t]+([^\s#]+)`)
var virtualHostPattern = regexp.MustCompile(`(?i)<VirtualHost\s+[^>]*:(\d+)>`)
var documentRootPattern = regexp.MustCompile(`(?i)^DocumentRoot\s+["']?([^"']+)["']?$`)
var identifierPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func NewLinuxBackend() *LinuxBackend {
	profile := loadApacheProfile(apacheProfileFile)
	return &LinuxBackend{
		runner: execRunner{}, service: profile.service, apacheBinary: profile.binary,
		apacheControl: profile.control, enableCommand: profile.enableCommand,
		disableCommand: profile.disableCommand, sitesAvailable: profile.sitesAvailable,
		sitesEnabled: profile.sitesEnabled, backupRoot: "/var/backups/aegisadmin-system/apache",
		apacheRoot: profile.configRoot,
	}
}

func (b *LinuxBackend) Execute(ctx context.Context, command, argument string) (map[string]any, *Error) {
	switch command {
	case "info":
		return b.info(ctx)
	case "overview":
		return b.overview(ctx)
	case "configtest":
		valid, message, err := b.configTest(ctx)
		return map[string]any{"valid": valid, "message": message}, err
	case "vhosts":
		return b.vhosts(ctx)
	case "config":
		return b.config(ctx, argument)
	case "sites":
		return b.sites()
	case "site":
		return b.site(argument)
	case "create":
		return b.create(ctx, argument)
	case "update":
		return b.update(ctx, argument)
	case "enable":
		return b.enable(ctx, argument)
	case "disable":
		return b.disable(ctx, argument)
	case "delete":
		return b.delete(ctx, argument)
	case "modules":
		return b.modules(ctx)
	case "reload":
		return b.reload(ctx)
	case "restart":
		return b.restart(ctx)
	}
	return nil, domainError(4, "COMMAND_NOT_FOUND", "La commande demandée n’existe pas dans le domaine apache.", nil)
}

func (b *LinuxBackend) info(ctx context.Context) (map[string]any, *Error) {
	result, err := b.runner.Run(ctx, b.apacheBinary, "-v")
	if err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, domainError(10, "APACHE_INFO_FAILED", "Les informations de version Apache sont indisponibles.", map[string]any{"output": result.Output})
	}
	version, built := "", ""
	for _, line := range strings.Split(result.Output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Server version:") {
			version = strings.TrimSpace(strings.TrimPrefix(line, "Server version:"))
		} else if strings.HasPrefix(line, "Server built:") {
			built = strings.TrimSpace(strings.TrimPrefix(line, "Server built:"))
		}
	}
	if version == "" {
		return nil, domainError(10, "INVALID_APACHE_INFO_RESPONSE", "La version Apache retournée est invalide.", nil)
	}
	return map[string]any{"version": version, "built": built}, nil
}

func (b *LinuxBackend) configTest(ctx context.Context) (bool, string, *Error) {
	result, err := b.runner.Run(ctx, b.apacheControl, "configtest")
	if err != nil {
		return false, "", err
	}
	return result.OK, result.Output, nil
}

func (b *LinuxBackend) modules(ctx context.Context) (map[string]any, *Error) {
	result, err := b.runner.Run(ctx, b.apacheControl, "-M")
	if err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, domainError(10, "APACHE_MODULES_FAILED", "La liste des modules Apache est indisponible.", map[string]any{"output": result.Output})
	}
	modules := []map[string]any{}
	for _, line := range strings.Split(result.Output, "\n") {
		match := modulePattern.FindStringSubmatch(strings.TrimSpace(line))
		if match != nil {
			modules = append(modules, map[string]any{"name": match[1], "type": strings.ToLower(match[2])})
		}
	}
	sort.Slice(modules, func(i, j int) bool {
		return strings.ToLower(modules[i]["name"].(string)) < strings.ToLower(modules[j]["name"].(string))
	})
	return map[string]any{"modules": modules}, nil
}

func (b *LinuxBackend) apacheS(ctx context.Context) (string, *Error) {
	result, err := b.runner.Run(ctx, b.apacheControl, "-S")
	if err != nil {
		return "", err
	}
	if !result.OK {
		return "", domainError(10, "APACHE_VHOSTS_FAILED", "La liste des VirtualHosts Apache est indisponible.", map[string]any{"output": result.Output})
	}
	return result.Output, nil
}

func configIdentifier(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = resolved
	}
	path, _ = filepath.Abs(path)
	digest := sha256.Sum256([]byte(path))
	return hex.EncodeToString(digest[:])
}

func (b *LinuxBackend) vhosts(ctx context.Context) (map[string]any, *Error) {
	output, err := b.apacheS(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	vhosts := []map[string]any{}
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		match := nameVHostPattern.FindStringSubmatch(line)
		serverName, configFile := "", ""
		port, configLine := 0, 0
		if match != nil {
			port, _ = strconv.Atoi(match[1])
			serverName, configFile = match[2], match[3]
			configLine, _ = strconv.Atoi(match[4])
		} else {
			match = defaultServerPattern.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			serverName, configFile = match[1], match[2]
			configLine, _ = strconv.Atoi(match[3])
		}
		key := serverName + "\x00" + strconv.Itoa(port) + "\x00" + configFile + "\x00" + strconv.Itoa(configLine)
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			vhosts = append(vhosts, map[string]any{
				"server_name": serverName, "port": port, "config_file": configFile,
				"config_id": configIdentifier(configFile), "config_line": configLine,
				"document_root": documentRoot(configFile, configLine),
			})
		}
	}
	sort.SliceStable(vhosts, func(i, j int) bool {
		left, right := strings.ToLower(vhosts[i]["server_name"].(string)), strings.ToLower(vhosts[j]["server_name"].(string))
		if left == right {
			return vhosts[i]["port"].(int) < vhosts[j]["port"].(int)
		}
		return left < right
	})
	return map[string]any{"virtual_hosts": vhosts}, nil
}

func documentRoot(path string, virtualHostLine int) any {
	if !filepath.IsAbs(path) {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	lines := []string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), maximumConfigSize)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	start := virtualHostLine - 1
	if start < 0 {
		start = 0
	}
	if start >= len(lines) {
		return nil
	}
	inside := false
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(lower, "<virtualhost") {
			inside = true
			continue
		}
		if inside && strings.HasPrefix(lower, "</virtualhost>") {
			break
		}
		if inside {
			match := documentRootPattern.FindStringSubmatch(trimmed)
			if match != nil {
				value := strings.TrimSpace(match[1])
				if value != "/" {
					value = strings.TrimRight(value, "/")
				}
				return value
			}
		}
	}
	return nil
}
