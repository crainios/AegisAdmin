package logs

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const logsPolicyFile = "/etc/aegisadmin-system/logs"
const logsRoot = "/var/log"

func defaultPolicy() ([]string, []string) {
	return []string{
		"/var/log/apache2", "/var/log/httpd", "/var/log/mysql",
		"/var/log/mariadb", "/var/log/php", "/var/log/nginx",
	}, []string{"/var/log/fail2ban.log"}
}

func loadPolicy(path string) ([]string, []string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return defaultPolicy()
	}
	directories, files := []string{}, []string{}
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
		kind, value, found := strings.Cut(line, "=")
		value = strings.TrimSpace(value)
		if !found || !safeConfiguredPath(value) {
			continue
		}
		switch strings.TrimSpace(kind) {
		case "directory":
			directories = append(directories, value)
		case "file":
			files = append(files, value)
		}
	}
	return uniquePaths(directories), uniquePaths(files)
}

func safeConfiguredPath(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == logsRoot {
		return false
	}
	relative, err := filepath.Rel(logsRoot, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func uniquePaths(paths []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, path := range paths {
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func safeResolvedPath(root, path string) (string, bool) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != filepath.Clean(path) {
		return "", false
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return resolved, true
}
