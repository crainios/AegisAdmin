package tor

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var torUnitPattern = regexp.MustCompile(`^tor(?:@[A-Za-z0-9_.-]+)?\.service$`)

func (b *Backend) loadProfile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "binary":
			if value == "/usr/sbin/tor" || value == "/usr/bin/tor" {
				b.binary = value
			}
		case "config":
			if safeTorConfig(value) {
				b.config = filepath.Clean(value)
			}
		case "defaults":
			if value == "" || safeTorConfig(value) || strings.HasPrefix(filepath.Clean(value), "/usr/share/tor/") {
				b.defaults = filepath.Clean(value)
				if value == "" {
					b.defaults = ""
				}
			}
		case "unit":
			if torUnitPattern.MatchString(value) {
				b.unit = value
				b.service = strings.TrimSuffix(value, ".service")
			}
		case "data_directory":
			clean := filepath.Clean(value)
			if clean == "/var/lib/tor" || strings.HasPrefix(clean, "/var/lib/tor-") {
				b.dataDirectory = clean
			}
		}
	}
}

func safeTorConfig(value string) bool {
	clean := filepath.Clean(value)
	return strings.HasPrefix(clean, "/etc/tor/") || strings.HasPrefix(clean, "/usr/local/etc/tor/")
}
