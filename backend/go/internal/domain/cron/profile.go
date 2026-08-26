package cron

import (
	"bufio"
	"context"
	"os"
	"strings"
)

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
		if value == "auto" {
			continue
		}
		switch strings.TrimSpace(key) {
		case "package":
			if value == "cron" || value == "cronie" {
				b.packageName = value
			}
		case "unit":
			if value == "cron.service" || value == "crond.service" {
				b.unit = value
				b.service = strings.TrimSuffix(value, ".service")
			}
		case "crontab":
			if value == "/usr/bin/crontab" || value == "/usr/local/bin/crontab" {
				b.paths.Crontab = value
			}
		case "spool_directory":
			if value == "/var/spool/cron/crontabs" || value == "/var/spool/cron" {
				b.paths.SpoolDirectory = value
			}
		case "system_crontab":
			if value == "/etc/crontab" {
				b.paths.SystemCrontab = value
			}
		case "cron_directory":
			if value == "/etc/cron.d" {
				b.paths.CronDirectory = value
			}
		case "anacron":
			if value == "/usr/sbin/anacron" || value == "/usr/bin/anacron" {
				b.paths.Anacron = value
			}
		case "anacrontab":
			if value == "/etc/anacrontab" {
				b.paths.Anacrontab = value
			}
		}
	}
}

func (b *Backend) packageVersion(ctx context.Context) string {
	if executable(b.paths.DpkgQuery) {
		value, status := b.runner.Run(ctx, b.paths.DpkgQuery, "--show", "--showformat=${Version}", "--", b.packageName)
		if status == 0 && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if executable(b.paths.RPM) {
		value, status := b.runner.Run(ctx, b.paths.RPM, "-q", "--qf", "%{EVR}", b.packageName)
		if status == 0 && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	value, status := b.runner.Run(ctx, b.paths.Crontab, "--version")
	if status == 0 {
		if line := strings.TrimSpace(strings.Split(value, "\n")[0]); line != "" {
			return line
		}
	}
	return ""
}
