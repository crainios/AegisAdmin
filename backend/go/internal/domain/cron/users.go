package cron

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const defaultMinimumUserID = 1000

type CronUser struct {
	Name   string `json:"name"`
	UID    int    `json:"uid"`
	System bool   `json:"system"`
}

type passwdEntry struct {
	name, home, shell string
	uid               int
}

func (b *Backend) users() ([]CronUser, error) {
	entries, err := readPasswd(b.paths.PasswdFile)
	if err != nil {
		return nil, err
	}
	minimumUID := readMinimumUID(b.paths.LoginDefsFile)
	explicit, err := readAllowedUsers(b.paths.AllowedUsersFile)
	if err != nil {
		return nil, err
	}
	denied, err := readAllowedUsers(b.paths.DeniedUsersFile)
	if err != nil {
		return nil, err
	}
	users := []CronUser{}
	for _, entry := range entries {
		if entry.name == "root" || entry.uid == 65534 {
			continue
		}
		if _, excluded := denied[entry.name]; excluded {
			continue
		}
		_, explicitlyAllowed := explicit[entry.name]
		human := entry.uid >= minimumUID && usableLogin(entry)
		if human || explicitlyAllowed {
			users = append(users, CronUser{Name: entry.name, UID: entry.uid, System: !human})
		}
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })
	return users, nil
}

func (b *Backend) editable(owner string) bool {
	users, err := b.users()
	if err != nil {
		return false
	}
	for _, candidate := range users {
		if candidate.Name == owner {
			return true
		}
	}
	return false
}

func readPasswd(path string) ([]passwdEntry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	entries := []passwdEntry{}
	for _, line := range strings.Split(string(content), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) != 7 || !safeName.MatchString(fields[0]) {
			return nil, errorsText("invalid passwd entry")
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil || uid < 0 {
			return nil, errorsText("invalid passwd uid")
		}
		entries = append(entries, passwdEntry{name: fields[0], uid: uid, home: fields[5], shell: fields[6]})
	}
	return entries, nil
}

func readMinimumUID(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return defaultMinimumUserID
	}
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "UID_MIN" {
			value, err := strconv.Atoi(fields[1])
			if err == nil && value > 0 {
				return value
			}
		}
	}
	return defaultMinimumUserID
}

func readAllowedUsers(path string) (map[string]struct{}, error) {
	if path == "" {
		return map[string]struct{}{}, nil
	}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]struct{}{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := map[string]struct{}{}
	for _, raw := range strings.Split(string(content), "\n") {
		value := strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
		if value == "" {
			continue
		}
		if !safeName.MatchString(value) || value == "root" {
			return nil, errorsText("invalid allowed user")
		}
		result[value] = struct{}{}
	}
	return result, nil
}

func usableLogin(entry passwdEntry) bool {
	if !filepath.IsAbs(entry.home) || !filepath.IsAbs(entry.shell) {
		return false
	}
	shell := filepath.Base(entry.shell)
	return shell != "nologin" && shell != "false"
}
