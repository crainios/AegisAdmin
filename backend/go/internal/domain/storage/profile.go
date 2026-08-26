package storage

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func (c *LinuxCollector) loadProfile(path string) {
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
		case "findmnt":
			if value != "auto" && allowedFindmnt(value) {
				c.findmnt = value
			}
		case "data_mount":
			if value == "none" {
				c.dataMount = ""
			} else if validMountPath(value) {
				c.dataMount = filepath.Clean(value)
			}
		}
	}
}

func allowedFindmnt(path string) bool {
	for _, candidate := range findmntCommandCandidates {
		if path == candidate {
			return true
		}
	}
	return false
}

func validMountPath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && path != "/" && !strings.ContainsAny(path, "\x00\r\n")
}
