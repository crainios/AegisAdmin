package apache

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const apacheProfileFile = "/etc/aegisadmin-system/apache"

var servicePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9@._-]*$`)

type apacheProfile struct {
	service, binary, control, configRoot, sitesAvailable, sitesEnabled string
	enableCommand, disableCommand                                      string
}

func defaultApacheProfile() apacheProfile {
	return apacheProfile{
		service: "apache2", binary: apacheCommand, control: apachectlCommand,
		configRoot: "/etc/apache2", sitesAvailable: "/etc/apache2/sites-available",
		sitesEnabled: "/etc/apache2/sites-enabled", enableCommand: a2ensiteCommand,
		disableCommand: a2dissiteCommand,
	}
}

func loadApacheProfile(path string) apacheProfile {
	profile := defaultApacheProfile()
	content, err := os.ReadFile(path)
	if err != nil {
		return profile
	}
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
		key, value, found := strings.Cut(line, "=")
		value = strings.TrimSpace(value)
		if !found || value == "" {
			continue
		}
		switch strings.TrimSpace(key) {
		case "service":
			if servicePattern.MatchString(value) {
				profile.service = value
			}
		case "binary":
			profile.binary = absoluteOr(profile.binary, value)
		case "control":
			profile.control = absoluteOr(profile.control, value)
		case "config_root":
			profile.configRoot = absoluteOr(profile.configRoot, value)
		case "sites_available":
			profile.sitesAvailable = absoluteOr(profile.sitesAvailable, value)
		case "sites_enabled":
			profile.sitesEnabled = absoluteOr(profile.sitesEnabled, value)
		case "enable_command":
			profile.enableCommand = absoluteOr(profile.enableCommand, value)
		case "disable_command":
			profile.disableCommand = absoluteOr(profile.disableCommand, value)
		}
	}
	return profile
}

func absoluteOr(fallback, value string) string {
	if filepath.IsAbs(value) && filepath.Clean(value) == value {
		return value
	}
	return fallback
}
