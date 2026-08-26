package configuration

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type LinuxPlatform struct{}

func NewLinuxPlatform() *LinuxPlatform { return &LinuxPlatform{} }

func (p *LinuxPlatform) Inventory(ctx context.Context) map[string]any {
	packages, packageManager := installedPackages(ctx)
	return map[string]any{
		"packages": map[string]any{"manager": packageManager, "count": len(packages), "items": packages},
		"services": map[string]any{"items": systemServices(ctx)},
		"accounts": accounts(),
		"ports":    map[string]any{"listeners": listeners(ctx)},
		"apache":   configurationFiles("/etc/apache2", "/etc/httpd"),
		"php":      configurationFiles("/etc/php", "/etc/php.d"),
		"mysql":    configurationFiles("/etc/mysql", "/etc/my.cnf.d"),
		"fail2ban": configurationFiles("/etc/fail2ban"),
		"cron":     configurationFiles("/etc/cron.d", "/etc/crontab"),
		"tor":      configurationFiles("/etc/tor"),
	}
}

func configurationFiles(roots ...string) []map[string]any {
	items := []map[string]any{}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry == nil || entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
				return nil
			}
			info, statErr := entry.Info()
			if statErr != nil || !info.Mode().IsRegular() || info.Size() > 2*1024*1024 {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			digest := sha256.Sum256(content)
			items = append(items, map[string]any{"path": path, "size": info.Size(), "mode": info.Mode().Perm().String(), "sha256": hex.EncodeToString(digest[:])})
			return nil
		})
	}
	return items
}

func systemServices(ctx context.Context) []map[string]string {
	if !executable("/usr/bin/systemctl") {
		return []map[string]string{}
	}
	activeOutput, _ := run(ctx, "/usr/bin/systemctl", "list-units", "--type=service", "--state=active", "--no-legend", "--no-pager")
	active := map[string]bool{}
	for _, line := range strings.Split(activeOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 0 {
			active[fields[0]] = true
		}
	}
	output, ok := run(ctx, "/usr/bin/systemctl", "list-unit-files", "--type=service", "--no-legend", "--no-pager")
	if !ok {
		return []map[string]string{}
	}
	items := []map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			state := "inactive"
			if active[fields[0]] {
				state = "active"
			}
			items = append(items, map[string]string{"unit": fields[0], "unit_file_state": fields[1], "state": state})
		}
	}
	return items
}

func installedPackages(ctx context.Context) ([]map[string]string, string) {
	if executable("/usr/bin/dpkg-query") {
		output, ok := run(ctx, "/usr/bin/dpkg-query", "-W", "-f=${binary:Package}\t${Version}\n")
		if ok {
			return parsePairs(output), "dpkg"
		}
	}
	if executable("/usr/bin/rpm") {
		output, ok := run(ctx, "/usr/bin/rpm", "-qa", "--qf", "%{NAME}\t%{VERSION}-%{RELEASE}.%{ARCH}\n")
		if ok {
			return parsePairs(output), "rpm"
		}
	}
	return []map[string]string{}, "unknown"
}

func parsePairs(output string) []map[string]string {
	items := []map[string]string{}
	for _, raw := range strings.Split(output, "\n") {
		parts := strings.SplitN(raw, "\t", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			items = append(items, map[string]string{"name": parts[0], "version": parts[1]})
		}
	}
	return items
}

func accounts() map[string]any {
	users := []map[string]any{}
	if file, err := os.Open("/etc/passwd"); err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			parts := strings.Split(scanner.Text(), ":")
			if len(parts) == 7 {
				uid, uidErr := strconv.Atoi(parts[2])
				gid, gidErr := strconv.Atoi(parts[3])
				if uidErr == nil && gidErr == nil {
					users = append(users, map[string]any{"name": parts[0], "uid": uid, "gid": gid, "home": parts[5], "shell": parts[6]})
				}
			}
		}
		file.Close()
	}
	groups := []map[string]any{}
	if file, err := os.Open("/etc/group"); err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			parts := strings.Split(scanner.Text(), ":")
			if len(parts) == 4 {
				if gid, err := strconv.Atoi(parts[2]); err == nil {
					members := []string{}
					if parts[3] != "" {
						members = strings.Split(parts[3], ",")
					}
					groups = append(groups, map[string]any{"name": parts[0], "gid": gid, "members": members})
				}
			}
		}
		file.Close()
	}
	return map[string]any{"user_count": len(users), "group_count": len(groups), "users": users, "groups": groups}
}

func listeners(ctx context.Context) []map[string]string {
	for _, binary := range []string{"/usr/bin/ss", "/usr/sbin/ss"} {
		if executable(binary) {
			output, ok := run(ctx, binary, "-H", "-lntu")
			if !ok {
				return []map[string]string{}
			}
			items := []map[string]string{}
			for _, line := range strings.Split(output, "\n") {
				if item, parsed := parseListener(line); parsed {
					items = append(items, item)
				}
			}
			return items
		}
	}
	return []map[string]string{}
}

func parseListener(line string) (map[string]string, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 5 {
		return nil, false
	}
	address, port, found := strings.Cut(fields[4], ":")
	if index := strings.LastIndex(fields[4], ":"); index >= 0 {
		address, port, found = fields[4][:index], fields[4][index+1:], true
	}
	if !found || address == "" || port == "" {
		return nil, false
	}
	address = strings.TrimPrefix(strings.TrimSuffix(address, "]"), "[")
	return map[string]string{"protocol": strings.ToLower(fields[0]), "state": strings.ToUpper(fields[1]), "address": address, "port": port}, true
}

func run(parent context.Context, binary string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	output, err := command.Output()
	return strings.TrimSpace(strings.ToValidUTF8(string(output), "�")), err == nil
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0
}
